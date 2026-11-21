package main

// Modelo de rivales. Cada rival tiene un alias estable durante todo el torneo,
// así que lo que aprendemos de él en una ronda sirve en las siguientes.

type perfil struct {
	manos    int // Manos en las que recibió cartas.
	vpipN    int // Manos en las que puso fichas voluntariamente preflop.
	pfrN     int // Manos en las que subió preflop.
	allinPre int // Manos en las que fue all-in preflop.

	postAgresivas int // Apuestas y subidas postflop.
	postPasivas   int // Checks y calls postflop.
	enfrento      int // Veces que enfrentó una apuesta postflop.
	foldeo        int // ... y se retiró.
}

// Las tasas mezclan lo observado con una creencia inicial ("prior") que pesa
// como unas pocas manos: con poca información no sacamos conclusiones bruscas.

func (p *perfil) vpip() float64 { return (0.32*8 + float64(p.vpipN)) / (8 + float64(p.manos)) }
func (p *perfil) pfr() float64  { return (0.16*8 + float64(p.pfrN)) / (8 + float64(p.manos)) }
func (p *perfil) tasaAllinPre() float64 {
	return (0.02*15 + float64(p.allinPre)) / (15 + float64(p.manos))
}

// agresividad: fracción de sus acciones postflop que son apuestas o subidas.
func (p *perfil) agresividad() float64 {
	return (0.28*10 + float64(p.postAgresivas)) / (10 + float64(p.postAgresivas+p.postPasivas+p.foldeo))
}

// foldAApuesta: qué tan seguido se retira cuando le apuestan postflop.
func (p *perfil) foldAApuesta() float64 {
	return (0.42*6 + float64(p.foldeo)) / (6 + float64(p.enfrento))
}

// farol: probabilidad estimada de que apueste sin tener nada.
func (p *perfil) farol() float64 {
	return limitar(0.08+1.1*(p.agresividad()-0.22), 0.05, 0.9)
}

func (b *bot) perfilDe(id string) *perfil {
	p := b.perfiles[id]
	if p == nil {
		p = &perfil{}
		b.perfiles[id] = p
	}
	return p
}

// aprenderDeMano actualiza los perfiles con el historial completo de una mano
// que acaba de terminar.
func (b *bot) aprenderDeMano(e *estadoJSON) {
	if e == nil || e.IDMano == b.manoAprendida {
		return
	}
	b.manoAprendida = e.IDMano
	for _, j := range e.Jugadores {
		if j.EnMano && j.ID != b.miID {
			b.perfilDe(j.ID).manos++
		}
	}
	vpip := map[string]bool{}
	pfr := map[string]bool{}
	allin := map[string]bool{}
	calle := ""
	hayApuesta := false
	for _, h := range e.Historial {
		if h.Etapa != calle {
			calle = h.Etapa
			hayApuesta = false
		}
		if h.IDJugador == b.miID {
			if t := h.Accion.Tipo; t == "bet" || t == "raise" || t == "allin" {
				hayApuesta = true
			}
			continue
		}
		p := b.perfilDe(h.IDJugador)
		t := h.Accion.Tipo
		if calle == "preflop" {
			switch t {
			case "call":
				vpip[h.IDJugador] = true
			case "bet", "raise":
				vpip[h.IDJugador] = true
				pfr[h.IDJugador] = true
			case "allin":
				vpip[h.IDJugador] = true
				pfr[h.IDJugador] = true
				allin[h.IDJugador] = true
			}
			continue
		}
		switch t {
		case "fold":
			if hayApuesta {
				p.enfrento++
				p.foldeo++
			}
		case "check":
			p.postPasivas++
		case "call":
			p.postPasivas++
			p.enfrento++
		case "bet", "raise", "allin":
			if hayApuesta {
				p.enfrento++
			}
			p.postAgresivas++
			hayApuesta = true
		}
	}
	for id := range vpip {
		b.perfilDe(id).vpipN++
	}
	for id := range pfr {
		b.perfilDe(id).pfrN++
	}
	for id := range allin {
		b.perfilDe(id).allinPre++
	}
}

func limitar(x, lo, hi float64) float64 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}
