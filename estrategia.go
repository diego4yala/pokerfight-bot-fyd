package main

import (
	"fmt"
	"math"
	"time"
)

// Estrategia: sólida y agresiva con las manos buenas, pensada para ganarle
// fichas a bots que cometen errores (pagan de más, se retiran de más o
// apuestan al azar) sin poner en juego todo el stack en jugadas parejas.
//
// decidir devuelve la jugada y una nota corta para el registro.

// Con cuánta equidad consideramos que "vamos adelante" según cuántos rivales
// quedan (índice = rivales).
var umbralValor = [...]float64{0.60, 0.60, 0.46, 0.38, 0.33, 0.30, 0.28, 0.26}
var umbralSubirPost = [...]float64{0.78, 0.78, 0.68, 0.60, 0.55, 0.52, 0.50, 0.48}

func porRivales(t []float64, n int) float64 {
	if n >= len(t) {
		n = len(t) - 1
	}
	return t[n]
}

func (b *bot) decidir(s *situacion, plazo time.Time) (accionJSON, string) {
	if len(s.rivales) == 0 {
		if s.validas["check"] {
			return accionJSON{Tipo: "check"}, "sin rivales"
		}
		return accionJSON{Tipo: "call"}, "sin rivales"
	}
	if s.preflop {
		return b.decidirPreflop(s, plazo)
	}
	return b.decidirPostflop(s, plazo)
}

// margenRiesgo: equidad extra que pedimos antes de arriesgar una parte grande
// del stack. En la clasificatoria quedarse sin fichas es definitivo; en la
// final gana uno solo y conviene jugar casi a valor esperado puro.
func (b *bot) margenRiesgo(s *situacion, fichas int64) float64 {
	if s.saldo <= 0 {
		return 0
	}
	riesgo := limitar(float64(fichas)/float64(s.saldo), 0, 1)
	// Arriesgar un stack corto pesa menos que arriesgar uno grande.
	if s.saldo < 15*s.bb {
		riesgo *= 0.4
	}
	if b.cfg.final {
		return 0.02 * riesgo
	}
	return 0.09 * riesgo
}

// ---------------------------------------------------------------- preflop --

func (b *bot) decidirPreflop(s *situacion, plazo time.Time) (accionJSON, string) {
	p := percentil(s.mias[0], s.mias[1])
	bb := s.bb
	n := len(s.rivales)
	corto := s.saldo <= 14*bb

	if s.subidasPre == 0 {
		// Nadie subió: abrimos con un rango que depende de cuántos faltan hablar.
		var umbral float64
		switch {
		case s.enMano <= 2:
			umbral = 0.70
		case s.porActuar >= 5:
			umbral = 0.14
		case s.porActuar == 4:
			umbral = 0.18
		case s.porActuar == 3:
			umbral = 0.26
		case s.porActuar == 2:
			umbral = 0.42
		case s.porActuar == 1:
			umbral = 0.36
		default: // Ciega grande con opción: solo subimos para castigar a los que pagaron.
			umbral = 0.13
		}
		umbral *= math.Pow(0.82, float64(s.limpers))
		if corto {
			// Stack corto: all-in o nada.
			if p <= umbral*0.9 && s.porActuar+s.limpers > 0 {
				return accionJSON{Tipo: "allin"}, fmt.Sprintf("corto, all-in p=%.2f", p)
			}
		} else if p <= umbral {
			total := bb*5/2 + int64(s.limpers)*bb
			if s.porActuar <= 1 && s.enMano > 2 {
				total += bb / 2 // Desde las ciegas, fuera de posición: más grande.
			}
			if b.rivalesSueltos(s) {
				total += bb // Si pagan con cualquier cosa, que paguen más.
			}
			return apostar(s, total), fmt.Sprintf("abrir p=%.2f", p)
		}
		if s.porIgualar == 0 {
			return accionJSON{Tipo: "check"}, "check gratis"
		}
		if !corto {
			barato := s.porIgualar <= bb/2
			par := s.mias[0].r == s.mias[1].r
			mismoPalo := s.mias[0].p == s.mias[1].p
			if barato && p <= 0.62 {
				return accionJSON{Tipo: "call"}, "completar ciega"
			}
			if s.limpers >= 1 && p <= 0.42 && (par || mismoPalo) && s.porActuar <= 2 {
				return accionJSON{Tipo: "call"}, "pagar barato con mano especulativa"
			}
		}
		return accionJSON{Tipo: "fold"}, fmt.Sprintf("fold p=%.2f", p)
	}

	// Alguien subió: comparamos nuestra equidad contra su rango con el precio.
	eq, sims := equidad(s.mias, nil, s.rangos(), b.cfg.sims, plazo, b.rnd)
	necesario := float64(s.porIgualar) / float64(s.pozo+s.porIgualar)
	valor := porRivales(umbralValor[:], n)
	nota := fmt.Sprintf("eq=%.2f nec=%.2f p=%.2f sims=%d", eq, necesario, p, sims)

	grande := float64(s.porIgualar) >= 0.3*float64(s.saldo) || s.porIgualar >= 25*bb || corto
	if grande {
		// Apuesta que compromete el stack: all-in o fold.
		if eq >= necesario+0.03+b.margenRiesgo(s, s.saldo) {
			if eq >= valor+0.03 && (s.validas["raise"] || s.porIgualar < s.saldo) && s.validas["allin"] {
				return accionJSON{Tipo: "allin"}, "all-in por valor " + nota
			}
			return accionJSON{Tipo: "call"}, "pagar apuesta grande " + nota
		}
		return accionJSON{Tipo: "fold"}, "fold a apuesta grande " + nota
	}

	extra := 0.0
	if s.subidasPre >= 2 {
		extra = 0.03
	}
	if eq >= valor+extra && (s.validas["raise"] || s.validas["allin"]) {
		// Resubir por valor: 3 veces la apuesta, más lo que ya hay de los que pagaron.
		total := s.e.ApuestaActual*3 + int64(n-1)*s.e.ApuestaActual/2
		if !s.enPosicion {
			total += s.e.ApuestaActual / 2
		}
		if float64(total) >= 0.4*float64(s.techo) {
			if eq >= valor+extra+0.04 {
				return accionJSON{Tipo: "allin"}, "all-in por valor " + nota
			}
			return accionJSON{Tipo: "call"}, "pagar (resubida comprometería el stack) " + nota
		}
		return apostar(s, total), "resubir por valor " + nota
	}
	margen := 0.05 + b.margenRiesgo(s, s.porIgualar)
	if !s.enPosicion {
		margen += 0.03
	}
	if s.subidasPre >= 2 {
		margen += 0.04
	}
	if s.porActuar > 0 {
		margen += 0.02 // Todavía pueden resubir detrás.
	}
	if eq >= necesario+margen && float64(s.porIgualar) <= 0.15*float64(s.saldo) {
		return accionJSON{Tipo: "call"}, "pagar " + nota
	}
	return accionJSON{Tipo: "fold"}, "fold " + nota
}

// rivalesSueltos: los rivales que quedan pagan con demasiadas manos.
func (b *bot) rivalesSueltos(s *situacion) bool {
	if len(s.rivales) == 0 {
		return false
	}
	suma := 0.0
	for _, r := range s.rivales {
		suma += r.perfil.vpip()
	}
	return suma/float64(len(s.rivales)) > 0.5
}

// --------------------------------------------------------------- postflop --

func (b *bot) decidirPostflop(s *situacion, plazo time.Time) (accionJSON, string) {
	n := len(s.rivales)
	eq, sims := equidad(s.mias, s.tablero, s.rangos(), b.cfg.sims, plazo, b.rnd)
	pozo := float64(s.pozo)
	saldo := float64(s.saldo)
	river := len(s.tablero) >= 5

	if s.porIgualar == 0 {
		// Nadie apostó: elegimos entre pasar y apostar comparando valor esperado.
		todosPueden := true
		foldMedio := 0.0
		estacion := false
		for _, r := range s.rivales {
			f := r.perfil.foldAApuesta()
			foldMedio += f
			if r.allin {
				todosPueden = false
			}
			if f < 0.25 {
				estacion = true
			}
		}
		foldMedio /= float64(n)
		encoger := 0.48 * limitar(foldMedio/0.42, 0.4, 1.3)
		eqSiPagan := eq - encoger*eq*(1-eq)
		esValor := eq >= porRivales(umbralValor[:], n)-0.02

		evPasar := eq * pozo
		mejorEV, mejorFrac := evPasar+0.04*pozo, 0.0
		for _, frac := range []float64{0.33, 0.6, 0.9, 1.4} {
			if frac > 1 && !(esValor && eq > 0.8 && estacion) {
				continue
			}
			monto := math.Min(frac*pozo, saldo)
			todosFold := 1.0
			if !todosPueden {
				todosFold = 0
			}
			for _, r := range s.rivales {
				todosFold *= limitar(r.perfil.foldAApuesta()*(0.55+0.75*frac), 0.02, 0.92)
			}
			ev := todosFold*pozo + (1-todosFold)*(eqSiPagan*(pozo+2*monto)-monto)
			if !esValor {
				// Farol o semi-farol: con límites estrictos.
				ok := n <= 2 && frac <= 0.6 && !estacion && todosPueden &&
					(s.yoAgresorPre || (s.enPosicion && !s.presionPost)) &&
					(b.cfg.final || monto <= 0.15*saldo)
				if river && eq > 0.25 {
					ok = false // Con algo de valor en el river, mejor ver las cartas gratis.
				}
				if !ok {
					continue
				}
			}
			if ev > mejorEV {
				mejorEV, mejorFrac = ev, frac
			}
		}
		nota := fmt.Sprintf("eq=%.2f sims=%d", eq, sims)
		if mejorFrac > 0 {
			if !esValor && b.rnd.f() > 0.7 {
				return pasar(s), "pasar (farol omitido) " + nota
			}
			monto := int64(mejorFrac * pozo)
			if monto < s.bb {
				monto = s.bb
			}
			tipo := "apostar por valor"
			if !esValor {
				tipo = "apostar de farol"
			}
			return apostar(s, s.yo.ApuestaRonda+monto), fmt.Sprintf("%s %.0f%% del pozo %s", tipo, mejorFrac*100, nota)
		}
		return pasar(s), "pasar " + nota
	}

	// Nos apostaron: precio contra equidad (ya filtrada por la apuesta rival).
	pagar := float64(s.porIgualar)
	necesario := pagar / (pozo + pagar)
	margen := 0.03 + b.margenRiesgo(s, s.porIgualar)
	nota := fmt.Sprintf("eq=%.2f nec=%.2f sims=%d", eq, necesario, sims)

	if eq >= porRivales(umbralSubirPost[:], n) && (s.validas["raise"] || s.validas["allin"]) {
		total := s.e.ApuestaActual + int64(0.75*(pozo+pagar))
		if float64(total-s.yo.ApuestaRonda) >= 0.5*saldo {
			if s.validas["allin"] {
				return accionJSON{Tipo: "allin"}, "all-in por valor " + nota
			}
			return accionJSON{Tipo: "call"}, "pagar " + nota
		}
		return apostar(s, total), "subir por valor " + nota
	}
	if eq >= necesario+margen {
		return accionJSON{Tipo: "call"}, "pagar " + nota
	}
	if !river && eq >= necesario-0.03 && pagar <= 0.08*saldo && pagar <= 0.5*pozo {
		return accionJSON{Tipo: "call"}, "pagar proyecto " + nota
	}
	return accionJSON{Tipo: "fold"}, "fold " + nota
}

func pasar(s *situacion) accionJSON {
	if s.validas["check"] {
		return accionJSON{Tipo: "check"}
	}
	return accionJSON{Tipo: "fold"}
}

// apostar arma una apuesta o subida hasta `total` fichas en la ronda.
// Recordatorio: monto es el TOTAL de la ronda, no el incremento.
func apostar(s *situacion, total int64) accionJSON {
	tipo := "raise"
	if s.e.ApuestaActual <= s.yo.ApuestaRonda {
		tipo = "bet"
	}
	return accionJSON{Tipo: tipo, Monto: total}
}

// sanear garantiza que lo que enviamos esté en acciones_validas y dentro de
// los límites de la mesa. Una acción inválida cuesta lo mismo que un timeout.
func sanear(a accionJSON, s *situacion) accionJSON {
	pasiva := func() accionJSON {
		switch {
		case s.validas["check"]:
			return accionJSON{Tipo: "check"}
		case s.validas["call"]:
			return accionJSON{Tipo: "call"}
		case s.validas["allin"] && s.porIgualar >= s.saldo:
			return accionJSON{Tipo: "allin"} // Pagar con todo lo que queda.
		}
		return accionJSON{Tipo: "fold"}
	}
	switch a.Tipo {
	case "fold":
		if s.validas["check"] {
			return accionJSON{Tipo: "check"} // Nunca retirarse gratis.
		}
		return accionJSON{Tipo: "fold"}
	case "check":
		if s.validas["check"] {
			return a
		}
		return accionJSON{Tipo: "fold"}
	case "call":
		if s.validas["call"] {
			return accionJSON{Tipo: "call"}
		}
		return pasiva()
	case "allin":
		if s.validas["allin"] {
			return accionJSON{Tipo: "allin"}
		}
		return pasiva()
	case "bet", "raise":
		tipo := ""
		if s.validas["bet"] {
			tipo = "bet"
		} else if s.validas["raise"] {
			tipo = "raise"
		}
		minimo := s.e.ApuestaActual + s.e.SubidaMinima
		total := a.Monto
		if total < minimo {
			total = minimo
		}
		if tipo == "" || total >= s.techo {
			if s.validas["allin"] && a.Monto >= s.techo*9/10 {
				return accionJSON{Tipo: "allin"}
			}
			if tipo == "" || minimo >= s.techo {
				return pasiva()
			}
			total = s.techo - 1
			if total < minimo {
				return pasiva()
			}
		}
		return accionJSON{Tipo: tipo, Monto: total}
	}
	return pasiva()
}
