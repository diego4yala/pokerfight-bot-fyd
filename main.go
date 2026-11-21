// Bot de poker del equipo F&D para el PokerFight (AICS PUCP).
//
// Uso:
//
//	bot -addr HOST:PUERTO -token TU_TOKEN
//
// Sin dependencias: solo la librería estándar de Go. Habla el protocolo
// v1.0.0 del kit (TCP, un JSON por línea), se reconecta solo si se cae la
// conexión y nunca envía una acción que no esté en acciones_validas.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

type config struct {
	addr    string
	token   string
	final   bool          // Modo final: menos aversión al riesgo.
	sims    int           // Tope de simulaciones por decisión.
	pensar  time.Duration // Tiempo máximo de simulación por decisión.
	detalle bool          // Registrar cada decisión.
	unaVez  bool          // No reconectar al terminar (para pruebas locales).
	archivo string        // Archivo de registro.
}

type bot struct {
	cfg        config
	miID       string
	cartas     [2]carta
	manoCartas string // id_mano para el que valen b.cartas.
	ultimo     *estadoJSON
	perfiles   map[string]*perfil
	rnd        *azar
	log        *log.Logger

	manoAprendida string
	manos         int
	saldo         int64
	mesa          string
	timeouts      int
}

func main() {
	var cfg config
	gen := flag.Bool("gen-tabla", false, "(desarrollo) imprime tabla_preflop.go")
	flag.StringVar(&cfg.addr, "addr", "localhost:9000", "dirección de la mesa (host:puerto)")
	flag.StringVar(&cfg.token, "token", "", "token del equipo (o variable POKER_TOKEN, o archivo token.txt)")
	modo := flag.String("modo", "clasificatoria", "clasificatoria (cuida el stack) o final (más agresivo)")
	flag.IntVar(&cfg.sims, "sims", 20000, "tope de simulaciones por decisión")
	pensar := flag.Int("pensar-ms", 400, "tiempo máximo de simulación por decisión, en ms")
	flag.BoolVar(&cfg.detalle, "detalle", false, "muestra cada decisión en pantalla")
	flag.BoolVar(&cfg.unaVez, "una-vez", false, "termina cuando la mesa cierra, sin reconectar")
	flag.StringVar(&cfg.archivo, "log", "bot.log", "archivo de registro (vacío = sin archivo)")
	flag.Bool("silencioso", false, "(compatibilidad con la arena del kit)")
	flag.Parse()
	if *gen {
		generarTabla()
		return
	}
	cfg.final = strings.HasPrefix(strings.ToLower(*modo), "f")
	cfg.pensar = time.Duration(*pensar) * time.Millisecond
	if cfg.token == "" {
		cfg.token = os.Getenv("POKER_TOKEN")
	}
	if cfg.token == "" {
		if datos, err := os.ReadFile("token.txt"); err == nil {
			cfg.token = strings.TrimSpace(string(datos))
		}
	}
	if cfg.token == "" {
		fmt.Fprintln(os.Stderr, "Falta el token: usa -token, la variable POKER_TOKEN o un archivo token.txt")
		os.Exit(2)
	}

	salida := io.Writer(os.Stderr)
	if cfg.archivo != "" {
		if f, err := os.OpenFile(cfg.archivo, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			defer f.Close()
			salida = io.MultiWriter(os.Stderr, f)
		}
	}
	b := &bot{
		cfg:      cfg,
		perfiles: map[string]*perfil{},
		rnd:      nuevoAzar(uint64(time.Now().UnixNano())),
		log:      log.New(salida, "", log.LstdFlags),
	}
	modoTxt := "clasificatoria"
	if cfg.final {
		modoTxt = "final"
	}
	b.log.Printf("bot F&D: conectando a %s (modo %s)", cfg.addr, modoTxt)
	b.correr()
}

var errDefinitivo = errors.New("error definitivo")

// correr mantiene al bot conectado: si la conexión se cae, vuelve a entrar con
// el mismo token y retoma su silla y sus fichas. La espera crece entre
// intentos para no lanzar ráfagas de conexiones contra el servidor.
func (b *bot) correr() {
	const esperaMinima = 250 * time.Millisecond
	espera := esperaMinima
	caidasRapidas := 0
	for {
		inicio := time.Now()
		saludado, err := b.sesion()
		duro := time.Since(inicio)
		if errors.Is(err, errDefinitivo) {
			b.log.Printf("no se puede continuar: %v", err)
			os.Exit(1)
		}
		if b.cfg.unaVez && saludado {
			b.log.Printf("la mesa cerró la conexión; fin (%d manos jugadas)", b.manos)
			return
		}
		switch {
		case saludado && duro >= 10*time.Second:
			// Veníamos jugando bien: volvemos lo antes posible, que cada mano
			// ausente cuesta ciegas.
			espera = esperaMinima
			caidasRapidas = 0
		case saludado:
			caidasRapidas++
			if caidasRapidas == 3 {
				b.log.Printf("AVISO: la conexión se corta apenas entra. ¿Hay otra copia del bot corriendo con el mismo token?")
			}
		}
		pausa := espera + time.Duration(b.rnd.n(int(espera/2)+1))
		b.log.Printf("desconectado (%v); reintento en %s", err, pausa.Round(10*time.Millisecond))
		time.Sleep(pausa)
		if espera *= 2; espera > 5*time.Second {
			espera = 5 * time.Second
		}
	}
}

// sesion abre una conexión, saluda y atiende mensajes hasta que se corte.
func (b *bot) sesion() (saludado bool, err error) {
	con, err := (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).Dial("tcp", b.cfg.addr)
	if err != nil {
		return saludado, err
	}
	defer con.Close()
	lector := bufio.NewReaderSize(con, 1<<16)
	enviar := func(m mensajeBot) error {
		datos, err := json.Marshal(m)
		if err != nil {
			return err
		}
		con.SetWriteDeadline(time.Now().Add(3 * time.Second))
		_, err = con.Write(append(datos, '\n'))
		return err
	}
	if err := enviar(mensajeBot{Tipo: "saludo", Version: versionProtocolo, Token: b.cfg.token}); err != nil {
		return saludado, err
	}
	// Sin plazo de lectura: entre rondas la conexión queda en silencio durante
	// minutos u horas, y eso es normal. El keepalive de TCP detecta un corte real.
	for {
		linea, err := leerLinea(lector)
		if err != nil {
			return saludado, err
		}
		var m mensajeMesa
		if json.Unmarshal(linea, &m) != nil {
			continue // Una línea que no entendemos no es motivo para caerse.
		}
		switch m.Tipo {
		case "bienvenida":
			b.miID = m.IDJugador
			if b.miID == "" {
				b.miID = b.cfg.token // Mesa local en modo abierto.
			}
			saludado = true
			b.log.Printf("conectado como %s", b.miID)
		case "error":
			texto := strings.ToLower(m.Mensaje)
			if strings.Contains(texto, "demasiadas conexiones") {
				return saludado, errors.New("mesa saturada")
			}
			if strings.Contains(texto, "credencial") || strings.Contains(texto, "versi") || strings.Contains(texto, "token") {
				return saludado, fmt.Errorf("%w: la mesa respondió %q", errDefinitivo, m.Mensaje)
			}
			return saludado, fmt.Errorf("la mesa respondió: %s", m.Mensaje)
		case "mano_inicio":
			b.manoInicio(&m)
		case "estado":
			if m.Estado != nil {
				b.ultimo = m.Estado
			}
		case "solicitar_accion":
			if m.Estado == nil {
				continue
			}
			b.ultimo = m.Estado
			accion := b.turno(&m)
			if err := enviar(mensajeBot{Tipo: "accion", Version: versionProtocolo, IDMano: m.Estado.IDMano, Accion: &accion}); err != nil {
				return saludado, err
			}
		case "mano_fin":
			b.manoFin(&m)
		}
	}
}

// leerLinea junta una línea completa aunque llegue en varios trozos.
func leerLinea(r *bufio.Reader) ([]byte, error) {
	var linea []byte
	for {
		trozo, err := r.ReadSlice('\n')
		linea = append(linea, trozo...)
		if errors.Is(err, bufio.ErrBufferFull) && len(linea) < 4<<20 {
			continue
		}
		return linea, err
	}
}

func (b *bot) manoInicio(m *mensajeMesa) {
	if m.Estado == nil {
		return
	}
	// Si la mano anterior no cerró con mano_fin, igual aprendemos de ella.
	if b.ultimo != nil && b.ultimo.IDMano != m.Estado.IDMano {
		b.aprenderDeMano(b.ultimo)
	}
	b.ultimo = m.Estado
	b.manoCartas = ""
	if len(m.Cartas) == 2 {
		c0, ok0 := aCarta(m.Cartas[0])
		c1, ok1 := aCarta(m.Cartas[1])
		if ok0 && ok1 {
			b.cartas = [2]carta{c0, c1}
			b.manoCartas = m.Estado.IDMano
		}
	}
	// Una mesa nueva (ronda nueva) se nota porque cambia el prefijo del id.
	mesa := m.Estado.IDMano
	if i := strings.LastIndex(mesa, "-mano-"); i > 0 {
		mesa = mesa[:i]
	} else {
		mesa = ""
	}
	if mesa != b.mesa {
		b.mesa = mesa
		for _, j := range m.Estado.Jugadores {
			if j.ID == b.miID {
				b.log.Printf("--- mesa nueva %s: %d jugadores, empiezo con %d fichas", mesa, len(m.Estado.Jugadores), j.Saldo+j.ApuestaRonda)
			}
		}
	}
}

func (b *bot) manoFin(m *mensajeMesa) {
	b.manos++
	if b.ultimo != nil && (m.Resultado == nil || m.Resultado.IDMano == b.ultimo.IDMano) {
		b.aprenderDeMano(b.ultimo)
	}
	if b.manos%10 == 0 && b.ultimo != nil {
		for _, j := range b.ultimo.Jugadores {
			if j.ID == b.miID {
				b.log.Printf("mano %d: ~%d fichas", b.manos, j.Saldo)
			}
		}
	}
}

// turno decide una jugada sin pasarse nunca del plazo y sin enviar nunca algo
// inválido. Si la estrategia falla o tarda, se juega la acción segura.
func (b *bot) turno(m *mensajeMesa) accionJSON {
	s := b.leerSituacion(m)
	segura := sanear(accionJSON{Tipo: "fold"}, s)
	if b.manoCartas != m.Estado.IDMano {
		// Reconectamos a mitad de mano y no sabemos nuestras cartas.
		b.log.Printf("%s: sin cartas conocidas (reconexión), juego seguro: %s", m.Estado.IDMano, segura.Tipo)
		return segura
	}
	plazoMesa := time.Duration(m.TimeoutMs) * time.Millisecond
	if plazoMesa <= 0 {
		plazoMesa = 5 * time.Second
	}
	// Margen amplio para la red: usamos como mucho un tercio del plazo.
	pensar := b.cfg.pensar
	if tope := plazoMesa / 3; pensar > tope {
		pensar = tope
	}
	inicio := time.Now()

	type resultado struct {
		a    accionJSON
		nota string
	}
	canal := make(chan resultado, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				canal <- resultado{segura, fmt.Sprintf("PÁNICO en la estrategia: %v", r)}
			}
		}()
		a, nota := b.decidir(s, inicio.Add(pensar))
		canal <- resultado{sanear(a, s), nota}
	}()
	var r resultado
	select {
	case r = <-canal:
	case <-time.After(pensar + plazoMesa/4):
		b.timeouts++
		r = resultado{segura, "la estrategia tardó demasiado: acción segura"}
		b.log.Printf("AVISO %s: %s", m.Estado.IDMano, r.nota)
	}
	if b.cfg.detalle {
		monto := ""
		if r.a.Monto > 0 {
			monto = fmt.Sprintf(" %d", r.a.Monto)
		}
		b.log.Printf("%s %s [%s|%s] pozo=%d pagar=%d rivales=%d -> %s%s (%s, %dms)",
			m.Estado.IDMano, m.Estado.Etapa, textoCartas(s.mias[:]), textoCartas(s.tablero), s.pozo, s.porIgualar,
			len(s.rivales), r.a.Tipo, monto, r.nota, time.Since(inicio).Milliseconds())
	}
	return r.a
}
