package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// generarTabla recalcula el ranking de las 169 manos iniciales (equidad contra
// 3 rivales al azar, con un ajuste chico por jugabilidad) e imprime el archivo
// tabla_preflop.go. Solo se usa al desarrollar: `bot -gen-tabla > tabla_preflop.go`.
func generarTabla() {
	type clase struct {
		i, j   int
		valor  float64
		combos float64
	}
	// Mientras se genera, todos los rangos rivales son "cualquier mano".
	rnd := nuevoAzar(20261122)
	var clases []clase
	lejos := time.Now().Add(time.Hour)
	for i := 0; i < 13; i++ {
		for j := 0; j < 13; j++ {
			var a, b carta
			combos := 12.0
			switch {
			case i == j:
				a, b = carta{uint8(i + 2), 0}, carta{uint8(i + 2), 1}
				combos = 6
			case i > j: // mismo palo
				a, b = carta{uint8(i + 2), 0}, carta{uint8(j + 2), 0}
				combos = 4
			default: // distinto palo: alta = j
				a, b = carta{uint8(j + 2), 0}, carta{uint8(i + 2), 1}
			}
			rivales := []rangoRival{{0, 1, 0, false, 0}, {0, 1, 0, false, 0}, {0, 1, 0, false, 0}}
			eq, _ := equidad([2]carta{a, b}, nil, rivales, 150000, lejos, rnd)
			hi, lo := int(a.r), int(b.r)
			if hi != lo {
				hueco := hi - lo
				if a.p == b.p {
					eq += 0.008
				}
				if hueco <= 2 && lo >= 5 {
					eq += 0.006
				}
				if a.p != b.p && hi >= 13 && lo <= 9 {
					eq -= 0.012 // Kicker dominado.
				}
			}
			clases = append(clases, clase{i, j, eq, combos})
		}
	}
	sort.Slice(clases, func(x, y int) bool { return clases[x].valor > clases[y].valor })
	var tabla [13][13]float64
	acum := 0.0
	for _, c := range clases {
		acum += c.combos
		tabla[c.i][c.j] = acum / 1326
	}
	var sb strings.Builder
	sb.WriteString("package main\n\n// Generado con `bot -gen-tabla`. No editar a mano.\n// pctPreflop[i][j]: fracción de manos iniciales iguales o mejores (0 = AA).\n// Diagonal = pares, i>j = mismo palo, i<j = distinto palo. Índice = rango-2.\nvar pctPreflop = [13][13]float64{\n")
	for i := 0; i < 13; i++ {
		sb.WriteString("\t{")
		for j := 0; j < 13; j++ {
			fmt.Fprintf(&sb, "%.4f", tabla[i][j])
			if j < 12 {
				sb.WriteString(", ")
			}
		}
		sb.WriteString("},\n")
	}
	sb.WriteString("}\n")
	fmt.Print(sb.String())
}
