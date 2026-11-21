package main

// Cartas y evaluador de manos. Mismas reglas que la mesa (docs/reglas.md):
// el as vale 14 salvo en la rueda A-2-3-4-5, y los palos no desempatan.

// carta: r = rango 2..14, p = palo 0..3.
type carta struct{ r, p uint8 }

const (
	catCartaAlta = iota
	catPar
	catDoblePar
	catTrio
	catEscalera
	catColor
	catFull
	catPoker
	catEscaleraColor
)

// evaluar devuelve un puntaje comparable (mayor = mejor) para 5 a 7 cartas.
// Con menos de 5 cartas sirve igual para saber la categoría del tablero.
// Formato: categoría<<20 | cinco desempates de 4 bits.
func evaluar(cs []carta) uint32 {
	var cuenta [15]uint8
	var porPalo [4]uint16
	var nPalo [4]uint8
	var mascara uint16
	for _, c := range cs {
		cuenta[c.r]++
		porPalo[c.p] |= 1 << c.r
		nPalo[c.p]++
		mascara |= 1 << c.r
	}
	for p := 0; p < 4; p++ {
		if nPalo[p] >= 5 {
			if h := altoEscalera(porPalo[p]); h > 0 {
				return catEscaleraColor<<20 | h<<16
			}
			return catColor<<20 | altas(porPalo[p], 5)
		}
	}
	var poker, trio1, trio2, par1, par2 uint32
	for r := uint32(14); r >= 2; r-- {
		switch cuenta[r] {
		case 4:
			poker = r
		case 3:
			if trio1 == 0 {
				trio1 = r
			} else if trio2 == 0 {
				trio2 = r
			}
		case 2:
			if par1 == 0 {
				par1 = r
			} else if par2 == 0 {
				par2 = r
			}
		}
	}
	if poker > 0 {
		return catPoker<<20 | poker<<16 | altas(mascara&^(1<<poker), 1)>>4
	}
	if trio1 > 0 && (trio2 > 0 || par1 > 0) {
		pareja := trio2
		if par1 > pareja {
			pareja = par1
		}
		return catFull<<20 | trio1<<16 | pareja<<12
	}
	if h := altoEscalera(mascara); h > 0 {
		return catEscalera<<20 | h<<16
	}
	if trio1 > 0 {
		return catTrio<<20 | trio1<<16 | altas(mascara&^(1<<trio1), 2)>>4
	}
	if par2 > 0 {
		return catDoblePar<<20 | par1<<16 | par2<<12 | altas(mascara&^(1<<par1|1<<par2), 1)>>8
	}
	if par1 > 0 {
		return catPar<<20 | par1<<16 | altas(mascara&^(1<<par1), 3)>>4
	}
	return catCartaAlta<<20 | altas(mascara, 5)
}

// altoEscalera devuelve la carta más alta de una escalera en la máscara de
// rangos, o 0 si no hay. La rueda (A-2-3-4-5) devuelve 5.
func altoEscalera(m uint16) uint32 {
	if m&(1<<14) != 0 {
		m |= 1 << 1
	}
	for h := uint32(14); h >= 5; h-- {
		if (m>>(h-4))&0x1F == 0x1F {
			return h
		}
	}
	return 0
}

// altas empaqueta los n rangos más altos de la máscara en nibbles, empezando
// en el bit 16 y bajando de a 4.
func altas(m uint16, n int) uint32 {
	var v uint32
	desp := uint(16)
	for r := uint32(14); r >= 2 && n > 0; r-- {
		if m&(1<<r) != 0 {
			v |= r << desp
			desp -= 4
			n--
		}
	}
	return v
}

func categoria(puntaje uint32) int { return int(puntaje >> 20) }

var letrasRango = "..23456789TJQKA"
var letrasPalo = "cdhs"

func (c carta) String() string {
	if c.r < 2 || c.r > 14 || c.p > 3 {
		return "??"
	}
	return string(letrasRango[c.r]) + string(letrasPalo[c.p])
}

func textoCartas(cs []carta) string {
	s := ""
	for _, c := range cs {
		s += c.String()
	}
	return s
}

// --- Azar rápido (xorshift64*) para las simulaciones ---

type azar struct{ s uint64 }

func nuevoAzar(semilla uint64) *azar {
	if semilla == 0 {
		semilla = 0x9E3779B97F4A7C15
	}
	return &azar{s: semilla}
}

func (a *azar) u64() uint64 {
	a.s ^= a.s >> 12
	a.s ^= a.s << 25
	a.s ^= a.s >> 27
	return a.s * 2685821657736338717
}

// n devuelve un entero en [0, n).
func (a *azar) n(n int) int { return int((a.u64() >> 33) % uint64(n)) }

// f devuelve un real en [0, 1).
func (a *azar) f() float64 { return float64(a.u64()>>11) / (1 << 53) }

// --- Clases de manos iniciales (las 169) ---

// claseDe ubica una mano inicial en la matriz 13x13: pares en la diagonal,
// del mismo palo arriba (fila = carta alta) y de distinto palo abajo.
func claseDe(a, b carta) (int, int) {
	hi, lo := int(a.r)-2, int(b.r)-2
	if lo > hi {
		hi, lo = lo, hi
	}
	if a.p == b.p {
		return hi, lo
	}
	return lo, hi
}

// percentil de una mano inicial: 0.0 = la mejor (AA), 1.0 = la peor.
// "Estar en el top 15%" es percentil(...) <= 0.15.
func percentil(a, b carta) float64 {
	i, j := claseDe(a, b)
	return pctPreflop[i][j]
}
