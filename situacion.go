package main

// situacion es la lectura de un turno: quién sigue en la mano, qué hizo cada
// uno y cuánto cuesta seguir. Se arma desde el estado público de la mesa.

type rivalMano struct {
	id       string
	saldo    int64
	allin    bool
	nivelPre int  // 0 nada, 1 pagó, 2 subió, 3 resubió, 4 all-in
	limp     bool // pagó la ciega sin que nadie hubiera subido
	actuoPre bool
	agresion int // presión postflop acumulada (apuesta = 1, subida = 2)
	pago     bool
	perfil   *perfil
}

type situacion struct {
	e       *estadoJSON
	yo      jugadorJSON
	mias    [2]carta
	tablero []carta
	rivales []rivalMano // Siguen en la mano (no se retiraron).
	validas map[string]bool

	preflop      bool
	enMano       int  // Jugadores que recibieron cartas.
	subidasPre   int  // Cantidad de subidas preflop hasta ahora.
	limpers      int  // Rivales que solo pagaron la ciega.
	porActuar    int  // Rivales que todavía no hablaron preflop.
	yoSubiPre    bool // Subí preflop en algún momento.
	yoAgresorPre bool // La última subida preflop fue mía.
	enPosicion   bool // Hablo último postflop.
	presionPost  bool // Algún rival apostó o subió postflop en esta mano.

	bb, pozo, porIgualar, saldo, techo int64
}

func (b *bot) leerSituacion(m *mensajeMesa) *situacion {
	e := m.Estado
	s := &situacion{e: e, mias: b.cartas, tablero: aCartas(e.Comunitarias), validas: map[string]bool{}}
	for _, v := range m.AccionesValidas {
		s.validas[v] = true
	}
	s.preflop = len(s.tablero) == 0
	s.bb = e.CiegaGrande
	if s.bb <= 0 {
		s.bb = 100
	}
	s.pozo = e.Pozo

	porID := map[string]*rivalMano{}
	orden := []string{}
	for _, j := range e.Jugadores {
		if j.EnMano {
			s.enMano++
		}
		if j.ID == b.miID {
			s.yo = j
			continue
		}
		if j.EnMano {
			porID[j.ID] = &rivalMano{id: j.ID, saldo: j.Saldo, allin: j.AllIn, perfil: b.perfilDe(j.ID)}
			if j.Activo {
				orden = append(orden, j.ID)
			}
		}
	}
	s.saldo = s.yo.Saldo
	s.techo = s.yo.Saldo + s.yo.ApuestaRonda
	s.porIgualar = e.ApuestaActual - s.yo.ApuestaRonda
	if s.porIgualar < 0 {
		s.porIgualar = 0
	}
	if s.porIgualar > s.saldo {
		s.porIgualar = s.saldo
	}

	calle := ""
	hayApuesta := false
	ultimoAgresor := ""
	for _, h := range e.Historial {
		if h.Etapa != calle {
			calle = h.Etapa
			hayApuesta = false
		}
		t := h.Accion.Tipo
		mio := h.IDJugador == b.miID
		r := porID[h.IDJugador]
		if calle == "preflop" {
			if r != nil {
				r.actuoPre = true
			}
			switch t {
			case "call":
				if r != nil {
					if s.subidasPre == 0 {
						r.limp = true
					}
					if r.nivelPre < 1 {
						r.nivelPre = 1
					}
				}
			case "bet", "raise", "allin":
				s.subidasPre++
				ultimoAgresor = h.IDJugador
				if mio {
					s.yoSubiPre = true
				} else if r != nil {
					r.limp = false
					switch {
					case t == "allin":
						r.nivelPre = 4
					case s.subidasPre == 1:
						r.nivelPre = 2
					default:
						r.nivelPre = 3
					}
				}
			}
			continue
		}
		switch t {
		case "bet", "raise", "allin":
			if r != nil {
				if hayApuesta {
					r.agresion += 2
				} else {
					r.agresion++
				}
				s.presionPost = true
			}
			hayApuesta = true
		case "call":
			if r != nil {
				r.pago = true
			}
		}
	}
	s.yoAgresorPre = ultimoAgresor == b.miID

	for _, id := range orden {
		r := porID[id]
		if r.limp {
			s.limpers++
		}
		if !r.actuoPre && !r.allin {
			s.porActuar++
		}
		s.rivales = append(s.rivales, *r)
	}

	// Posición postflop: se habla desde la izquierda del botón; quien queda
	// más lejos del botón en ese orden habla último.
	n := 0
	for _, j := range e.Jugadores {
		if j.PosicionSilla+1 > n {
			n = j.PosicionSilla + 1
		}
	}
	if n > 0 {
		dist := func(silla int) int { return ((silla-e.PosicionBoton-1)%n + n) % n }
		s.enPosicion = true
		yo := dist(s.yo.PosicionSilla)
		for _, j := range e.Jugadores {
			if j.ID != b.miID && j.Activo && j.EnMano && !j.AllIn && dist(j.PosicionSilla) > yo {
				s.enPosicion = false
			}
		}
	}
	return s
}

// rangos traduce lo que hizo cada rival en un rango de manos creíbles.
func (s *situacion) rangos() []rangoRival {
	out := make([]rangoRival, 0, len(s.rivales))
	for i := range s.rivales {
		r := &s.rivales[i]
		p := r.perfil
		rr := rangoRival{desde: 0, hasta: 1, agresion: r.agresion, pago: r.pago, farol: p.farol()}
		switch r.nivelPre {
		case 0:
			if r.actuoPre { // Ciega grande que pasó: no tiene una mano fuerte.
				rr.desde = 0.08
			}
		case 1:
			if r.limp {
				rr.desde, rr.hasta = 0.06, limitar(p.vpip()*1.1, 0.25, 1)
			} else {
				rr.desde, rr.hasta = 0.02, limitar(p.vpip()*0.8, 0.12, 1)
			}
		case 2:
			rr.hasta = limitar(p.pfr()*1.15, 0.05, 1)
		case 3:
			rr.hasta = limitar(p.pfr()*0.4, 0.025, 1)
		case 4:
			rr.hasta = limitar(maxf(p.tasaAllinPre()*1.6, p.pfr()*0.3), 0.03, 1)
		}
		if rr.agresion > 2 {
			rr.agresion = 2
		}
		out = append(out, rr)
	}
	return out
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
