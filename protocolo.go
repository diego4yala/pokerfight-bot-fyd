package main

// Mensajes del protocolo Mesa <-> Bot v1.0.0 (docs/protocolo.md del kit).
// Un JSON por línea. Los campos y tipos que no conocemos se ignoran.

const versionProtocolo = "1.0.0"

type cartaJSON struct {
	Rango int `json:"rango"`
	Palo  int `json:"palo"`
}

type accionJSON struct {
	Tipo  string `json:"tipo"`
	Monto int64  `json:"monto,omitempty"`
}

type jugadorJSON struct {
	ID            string `json:"id"`
	Saldo         int64  `json:"saldo"`
	ApuestaRonda  int64  `json:"apuesta_ronda"`
	Activo        bool   `json:"activo"`
	EnTorneo      bool   `json:"en_torneo"`
	AllIn         bool   `json:"allin"`
	EnMano        bool   `json:"en_mano"`
	PosicionSilla int    `json:"posicion_silla"`
}

type accionRegistrada struct {
	IDJugador string     `json:"id_jugador"`
	Etapa     string     `json:"etapa"`
	Accion    accionJSON `json:"accion"`
}

type estadoJSON struct {
	IDMano        string             `json:"id_mano"`
	Etapa         string             `json:"etapa"`
	Comunitarias  []cartaJSON        `json:"comunitarias"`
	Pozo          int64              `json:"pozo"`
	ApuestaActual int64              `json:"apuesta_actual"`
	SubidaMinima  int64              `json:"subida_minima"`
	CiegaChica    int64              `json:"ciega_chica"`
	CiegaGrande   int64              `json:"ciega_grande"`
	PosicionBoton int                `json:"posicion_boton"`
	Jugadores     []jugadorJSON      `json:"jugadores"`
	Historial     []accionRegistrada `json:"historial_acciones"`
}

type repartoJSON struct {
	IDJugador string `json:"id_jugador"`
	Monto     int64  `json:"monto"`
}

type resultadoJSON struct {
	IDMano       string                 `json:"id_mano"`
	Comunitarias []cartaJSON            `json:"comunitarias"`
	Repartos     []repartoJSON          `json:"repartos"`
	Mostradas    map[string][]cartaJSON `json:"mostradas"`
	Descripcion  map[string]string      `json:"descripcion"`
}

type mensajeMesa struct {
	Tipo            string         `json:"tipo"`
	Estado          *estadoJSON    `json:"estado"`
	Cartas          []cartaJSON    `json:"cartas"`
	AccionesValidas []string       `json:"acciones_validas"`
	TimeoutMs       int            `json:"timeout_ms"`
	Resultado       *resultadoJSON `json:"resultado"`
	Mensaje         string         `json:"mensaje"`
	IDJugador       string         `json:"id_jugador"`
	Silla           *int           `json:"silla"`
}

type mensajeBot struct {
	Tipo    string      `json:"tipo"`
	Version string      `json:"version"`
	Token   string      `json:"token,omitempty"`
	IDMano  string      `json:"id_mano,omitempty"`
	Accion  *accionJSON `json:"accion,omitempty"`
}

func aCarta(c cartaJSON) (carta, bool) {
	if c.Rango < 2 || c.Rango > 14 || c.Palo < 0 || c.Palo > 3 {
		return carta{}, false
	}
	return carta{uint8(c.Rango), uint8(c.Palo)}, true
}

func aCartas(cs []cartaJSON) []carta {
	out := make([]carta, 0, len(cs))
	for _, c := range cs {
		if k, ok := aCarta(c); ok {
			out = append(out, k)
		}
	}
	return out
}
