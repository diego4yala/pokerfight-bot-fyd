package main

import "time"

// rangoRival describe lo que creemos que puede tener un rival en esta mano.
type rangoRival struct {
	// Intervalo de percentiles de su mano inicial (0 = AA). Sale de lo que
	// hizo preflop y de qué tan suelto juega normalmente.
	desde, hasta float64
	// Presión postflop: 0 = nada, 1 = apostó, 2 = subió o apostó varias calles.
	agresion int
	// Pagó apuestas postflop (descarta el aire total).
	pago bool
	// Probabilidad de que apueste sin nada (0.05 = casi nunca, 0.9 = al azar).
	farol float64
}

// fuerzaTablero resume qué tan fuerte es una mano de dos cartas sobre el
// tablero actual, en una escala 0..1. Se usa para filtrar el rango de un rival
// que apostó: quien apuesta suele tener al menos un par decente o un proyecto.
type contextoTablero struct {
	tablero  []carta
	catTab   int
	ptsTab   uint32
	altaTab  uint8
	segunda  uint8
	hayRiver bool
}

func nuevoContexto(tablero []carta) contextoTablero {
	c := contextoTablero{tablero: tablero, hayRiver: len(tablero) >= 5}
	if len(tablero) == 0 {
		return c
	}
	c.ptsTab = evaluar(tablero)
	c.catTab = categoria(c.ptsTab)
	for _, k := range tablero {
		if k.r > c.altaTab {
			c.segunda = c.altaTab
			c.altaTab = k.r
		} else if k.r > c.segunda && k.r != c.altaTab {
			c.segunda = k.r
		}
	}
	return c
}

func (c *contextoTablero) fuerza(a, b carta) float64 {
	var buf [7]carta
	n := copy(buf[:], c.tablero)
	buf[n], buf[n+1] = a, b
	pts := evaluar(buf[:n+2])
	cat := categoria(pts)
	if cat > c.catTab {
		switch {
		case cat >= catEscalera:
			return 1.0
		case cat == catTrio:
			return 0.95
		case cat == catDoblePar:
			if c.catTab == catCartaAlta {
				return 0.9
			}
			// Tablero con par: lo nuestro vale como un par.
			return c.fuerzaPar(a, b)
		case cat == catPar:
			return c.fuerzaPar(a, b)
		}
	}
	// No mejora la categoría del tablero.
	if cat >= catEscalera && pts > c.ptsTab {
		return 0.6
	}
	if !c.hayRiver {
		if c.proyectoColor(a, b) {
			return 0.45
		}
		if c.proyectoEscalera(a, b) {
			return 0.40
		}
	}
	if a.r > c.altaTab && b.r > c.altaTab && !c.hayRiver {
		return 0.2
	}
	if a.r == 14 || b.r == 14 {
		return 0.15
	}
	return 0.06
}

// fuerzaPar valora un par hecho con nuestras cartas según contra qué carta
// del tablero queda.
func (c *contextoTablero) fuerzaPar(a, b carta) float64 {
	if a.r == b.r { // Par de mano.
		if a.r > c.altaTab {
			return 0.82
		}
		if a.r > c.segunda {
			return 0.55
		}
		return 0.38
	}
	par, kicker := uint8(0), uint8(0)
	for _, k := range c.tablero {
		if k.r == a.r && a.r > par {
			par, kicker = a.r, b.r
		}
		if k.r == b.r && b.r > par {
			par, kicker = b.r, a.r
		}
	}
	switch {
	case par == 0:
		return 0.15
	case par == c.altaTab:
		if kicker >= 11 {
			return 0.8
		}
		return 0.7
	case par == c.segunda:
		return 0.5
	}
	return 0.35
}

func (c *contextoTablero) proyectoColor(a, b carta) bool {
	var n [4]int
	for _, k := range c.tablero {
		n[k.p]++
	}
	n[a.p]++
	n[b.p]++
	return n[a.p] >= 4 || n[b.p] >= 4
}

func (c *contextoTablero) proyectoEscalera(a, b carta) bool {
	var m uint16
	for _, k := range c.tablero {
		m |= 1 << k.r
	}
	antes := m
	m |= 1<<a.r | 1<<b.r
	if m&(1<<14) != 0 {
		m |= 1 << 1
	}
	if antes&(1<<14) != 0 {
		antes |= 1 << 1
	}
	for bajo := uint(1); bajo <= 11; bajo++ {
		if (m>>bajo)&0xF == 0xF && (antes>>bajo)&0xF != 0xF {
			return true
		}
	}
	return false
}

// aceptar decide si una mano sorteada para el rival es creíble con lo que hizo.
func (r *rangoRival) aceptar(a, b carta, ctx *contextoTablero, rnd *azar) bool {
	p := percentil(a, b)
	if p < r.desde || p > r.hasta {
		// Fuga pequeña: nadie juega exactamente un rango de libro.
		if rnd.f() > 0.08 {
			return false
		}
	}
	if len(ctx.tablero) == 0 || (r.agresion == 0 && !r.pago) {
		return true
	}
	f := ctx.fuerza(a, b)
	prob := 1.0
	if r.agresion > 0 {
		fuerte := r.agresion >= 2
		switch {
		case f >= 0.75:
			prob = 1
		case f >= 0.45:
			prob = 0.55
			if fuerte {
				prob = 0.28
			}
		case f >= 0.3:
			prob = 0.3
			if fuerte {
				prob = 0.12
			}
		default:
			prob = r.farol
			if fuerte {
				prob = r.farol * 0.6
			}
		}
		if prob < r.farol {
			prob = r.farol
		}
	} else if r.pago && f < 0.2 {
		prob = 0.3
	}
	return prob >= 1 || rnd.f() < prob
}

// equidad estima por Monte Carlo la probabilidad de ganar la mano en el
// showdown (los empates cuentan en proporción) contra los rangos dados.
// Corta al llegar a maxSims o al plazo, lo que pase primero.
func equidad(mias [2]carta, tablero []carta, rivales []rangoRival, maxSims int, plazo time.Time, rnd *azar) (float64, int) {
	var usado [15][4]bool
	usado[mias[0].r][mias[0].p] = true
	usado[mias[1].r][mias[1].p] = true
	for _, c := range tablero {
		usado[c.r][c.p] = true
	}
	mazo := make([]carta, 0, 52)
	for r := uint8(2); r <= 14; r++ {
		for p := uint8(0); p < 4; p++ {
			if !usado[r][p] {
				mazo = append(mazo, carta{r, p})
			}
		}
	}
	ctx := nuevoContexto(tablero)
	nTab := len(tablero)
	var mano7, rival7 [7]carta
	copy(mano7[:], tablero)
	copy(rival7[:], tablero)
	L := len(mazo)

	ganado := 0.0
	sims := 0
	for sims < maxSims {
		if sims&127 == 127 && time.Now().After(plazo) {
			break
		}
		pos := 0
		// Cartas de cada rival, filtradas por su rango.
		for i := range rivales {
			r := &rivales[i]
			for intento := 0; ; intento++ {
				a := pos + rnd.n(L-pos)
				mazo[pos], mazo[a] = mazo[a], mazo[pos]
				b := pos + 1 + rnd.n(L-pos-1)
				mazo[pos+1], mazo[b] = mazo[b], mazo[pos+1]
				if intento >= 14 || r.aceptar(mazo[pos], mazo[pos+1], &ctx, rnd) {
					break
				}
			}
			pos += 2
		}
		// Completar el tablero.
		for k := nTab; k < 5; k++ {
			a := pos + rnd.n(L-pos)
			mazo[pos], mazo[a] = mazo[a], mazo[pos]
			mano7[k] = mazo[pos]
			rival7[k] = mazo[pos]
			pos++
		}
		mano7[5], mano7[6] = mias[0], mias[1]
		mio := evaluar(mano7[:])
		empatados := 1
		gano := true
		for i := range rivales {
			rival7[5], rival7[6] = mazo[2*i], mazo[2*i+1]
			suyo := evaluar(rival7[:])
			if suyo > mio {
				gano = false
				break
			}
			if suyo == mio {
				empatados++
			}
		}
		if gano {
			ganado += 1.0 / float64(empatados)
		}
		sims++
	}
	if sims == 0 {
		return 0.5 / float64(len(rivales)+1), 0
	}
	return ganado / float64(sims), sims
}
