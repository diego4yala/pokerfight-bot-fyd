# Bot F&D — PokerFight

Bot de **Texas Hold'em No-Limit** del equipo F&D para el PokerFight (AICS PUCP).
Está escrito en Go y solo usa la librería estándar: no tiene dependencias externas, claves ni servicios de terceros.

| Opción | Para qué sirve |
| --- | --- |
| `-addr` | Dirección de la mesa (`host:puerto`). |
| `-token` | Token del equipo. También se puede dar con la variable `POKER_TOKEN` o en un archivo `token.txt` junto al ejecutable. **El token no está en el repositorio** y `token.txt` está en `.gitignore`. |
| `-modo final` | Modo para la mesa final: arriesga más (ver *Cuidado del stack*). Por defecto es `clasificatoria`. |
| `-detalle` | Muestra en pantalla cada decisión con su razonamiento (equidad, precio, simulaciones). |
| `-pensar-ms` | Tiempo máximo de simulación por decisión (400 ms por defecto). |
| `-sims` | Tope de simulaciones Monte Carlo por decisión (20 000 por defecto). |
| `-log` | Archivo de registro (`bot.log` por defecto; vacío para no guardar). |
| `-una-vez` | Termina cuando la mesa cierra, sin reconectar. Útil para pruebas locales. |

---

## La mecánica del juego

### Las reglas de la mesa

- Texas Hold'em No-Limit en mesas de 5 o 6 bots. Ciegas fijas de 50/100 y 10 000 fichas iniciales, es decir, 100 ciegas grandes.
- Cada jugador recibe 2 cartas privadas. Se reparten 5 comunitarias en tres calles: flop (3), turn (1) y river (1). Hay una ronda de apuestas antes del flop y otra después de cada calle.
- En su turno el bot puede retirarse (`fold`), pasar (`check`), igualar (`call`), apostar (`bet`), subir (`raise`) o ir con todo (`allin`). La mesa indica en cada turno cuáles de esas acciones son válidas.
- Hay 5 segundos por jugada. Una acción inválida o fuera de tiempo se castiga igual que retirarse.

### El formato del torneo y lo que implica

- **Clasificatoria:** 10 rondas de hasta 50 manos o 6 minutos. **Las fichas se conservan de una ronda a otra** y quien se queda en cero queda eliminado. Pasan a la final los 6 con más fichas.
- **Final:** gana el último en pie o quien tenga más fichas al cabo de una hora.

De ahí salen las tres ideas que guían el diseño:

1. **No basta con sobrevivir.** Las ciegas cuestan unas 30 fichas por mano, unas 1 500 en toda la clasificatoria. Un bot que se retira siempre queda fuera, así que hay que ganar fichas activamente.
2. **Quebrar es definitivo.** En la clasificatoria se pide una ventaja extra antes de arriesgar una parte grande del stack. En la final, donde solo gana uno, ese margen casi desaparece.
3. **Los rivales son bots de otros estudiantes.** Lo más probable son bots que pagan casi todo, que suben al azar o que copian los ejemplos del kit. Rinde más un estilo sólido que les cobra esos errores (apostar fuerte las manos buenas y farolear poco) que un estilo de equilibrio teórico. Además, cada rival conserva su alias todo el torneo, así que se puede aprender cómo juega cada uno.

---

## Cómo se construyó

1. **Análisis de las reglas y del kit.** Se leyeron el protocolo, las reglas y el formato de competencia, y se identificaron los errores técnicos que más fichas cuestan:
   - `monto` es el **total** apostado en la ronda, no el incremento.
   - Si la subida mínima no entra en el stack, hay que declarar `allin`.
   - El bot se identifica por su `id_jugador`, no por el token.
   - El bot debe reconectarse solo y soportar horas de silencio entre rondas.
2. **Línea base.** Se jugó un torneo entre los 7 bots de ejemplo del kit con las ciegas reales. El mejor fue `darwin` (algoritmo genético), seguido de `estadistico` (Monte Carlo) y `espia` (modelado de rivales). Se tomó como objetivo superarlos.
3. **Diseño.** La estrategia combina las tres ideas que mejor funcionaban en el kit:
   - Un ranking precalculado de manos iniciales.
   - Simulación Monte Carlo, pero contra el **rango probable** de cada rival y no contra cartas al azar.
   - Un modelo de cada rival que aprende de una ronda a otra.
4. **Banco de pruebas.** Se automatizó la simulación de mesas con el formato de la clasificatoria (6 bots, 50 manos, 10 000 fichas, 50/100) contra los bots del kit. También se probaron cortes de conexión a mitad de partida.
5. **Ajuste.** Con los registros de decisiones se revisaron las manos en las que el bot perdía el stack y se calibraron los umbrales de riesgo y los tiempos de reconexión.

### Resultados de las pruebas locales

| Prueba | Resultado |
| --- | --- |
| 60 mesas de 6 bots contra los bots del kit, formato clasificatoria | Promedio de **16 258 fichas** (empezando con 10 000), el más alto. El segundo fue `darwin`, con 13 400. |
| Duelos uno contra uno con la arena del kit (8 bots) | 2.º puesto, 11 duelos ganados de 14, **0 timeouts**. |
| Corte de conexión a mitad de partida | Se reconectó solo y recuperó su silla con sus fichas. |
| Tiempo por decisión | Entre 5 y 50 ms, muy por debajo de los 5 segundos del límite. |

---

## Cómo decide el bot

### 1. Antes del flop

- **Ranking de manos iniciales.** Las 169 manos iniciales posibles (pares, del mismo palo y de distinto palo) se ordenan según su equidad contra 3 rivales con cartas al azar. Ese valor se ajusta un poco por jugabilidad: bonus para manos del mismo palo y conectadas, castigo para manos con kicker débil. El resultado es el percentil de cada mano (0 = AA, 1 = la peor), en `tabla_preflop.go`, que se genera con `fyd-bot -gen-tabla`.
- **Si nadie subió**, abre con un rango que se ensancha según la posición: el 14 % mejor de las manos si quedan 5 por hablar, el 42 % en el botón y el 70 % mano a mano. El rango se cierra si ya hay jugadores que solo pagaron la ciega. La subida es de 2,5 ciegas grandes más una por cada uno de esos jugadores, y es mayor si los rivales pagan demasiado.
- **Si alguien subió**, estima qué manos puede tener ese rival según lo que hizo: pagar, subir, resubir o ir all-in, ajustado por lo suelto que juega normalmente. Luego simula la equidad contra ese rango y la compara con el precio de pagar:
  - **Resube por valor** si la equidad supera el umbral para el número de rivales en la mano.
  - **Paga** si la equidad supera el precio más un margen, que crece si está fuera de posición, si ya hubo varias subidas o si quedan rivales por hablar.
  - **Se retira** en cualquier otro caso.
- **Con 14 ciegas grandes o menos**, juega all-in o se retira, sin términos medios.

### 2. Después del flop

- **Equidad por Monte Carlo.** Hasta 20 000 simulaciones (o 400 ms) en las que se reparten cartas a cada rival y se completa el tablero. Las cartas de cada rival se **filtran por sus acciones**: quien apostó o subió rara vez tiene aire (salvo que su historial muestre que apuesta al azar), y quien solo pagó no suele tener nada. Las manos se comparan con un evaluador de 5 a 7 cartas con las mismas reglas que la mesa, incluida la escalera A-2-3-4-5.
- **Si nadie apostó**, calcula el valor esperado de pasar y el de apostar un 33 %, 60 % o 90 % del pozo (o un 140 % contra rivales que pagan todo y con una mano muy fuerte), y elige el mayor. Ese cálculo tiene en cuenta qué tan seguido se retira cada rival ante una apuesta.
- **Faroles con límites estrictos:** solo contra uno o dos rivales, nunca contra quien paga todo, de no más del 60 % del pozo y, en la clasificatoria, como mucho el 15 % del stack. Solo los hace si fue el último en subir antes del flop o si está en posición sin haber recibido presión.
- **Si le apostaron**, compara la equidad con el precio:
  - **Sube** con manos muy fuertes. Si la subida compromete la mitad del stack, va all-in.
  - **Paga** si la equidad cubre el precio más el margen de riesgo.
  - **Paga barato con proyectos** de color o de escalera antes del river.
  - **Se retira** en cualquier otro caso.

### 3. Modelo de rivales

Por cada alias, el bot guarda:

- Con qué frecuencia entra en el pozo (VPIP).
- Con qué frecuencia sube antes del flop (PFR).
- Con qué frecuencia va all-in antes del flop.
- Qué fracción de sus acciones después del flop son apuestas o subidas.
- Con qué frecuencia se retira cuando le apuestan.

Cada estadística se mezcla con un valor inicial razonable que pesa como unas pocas manos, para no sacar conclusiones con poca información. Como el alias se mantiene todo el torneo, lo aprendido en una ronda se usa en las siguientes. Estas estadísticas ajustan:

- El rango de manos que se le atribuye al rival.
- La probabilidad de que esté faroleando.
- El tamaño de las apuestas por valor.
- Si vale la pena farolear contra él.

### 4. Cuidado del stack

Antes de arriesgar fichas, el bot pide una equidad extra proporcional a la fracción del stack en juego: hasta 9 puntos porcentuales en la clasificatoria y solo 2 en modo final. Con un stack corto ese margen se reduce, porque arriesgarlo cuesta menos.

---

## Robustez técnica

- **Nunca envía una acción inválida.** Cada decisión pasa por un validador final (`sanear`) que la ajusta a `acciones_validas`, respeta la subida mínima, convierte en `allin` lo que no entra en el stack y nunca se retira si puede pasar gratis.
- **Nunca se pasa del tiempo.** La estrategia corre con un plazo de, como mucho, un tercio del límite. Si fallara o tardara, se envía una acción segura antes de que venza.
- **Reconexión automática** con el mismo token: la espera crece de 0,25 a 5 segundos, sin ráfagas contra el servidor, y avisa si detecta otra copia del bot con el mismo token.
- **Soporta horas de silencio** entre rondas: no corta la conexión por inactividad y usa *keepalive* de TCP para detectar cortes reales.
- Si se reconecta a mitad de una mano y no conoce sus cartas, juega la acción más segura.

---

## Estructura del código

| Archivo | Contenido |
| --- | --- |
| `main.go` | Conexión, reconexión, control del plazo y registro. |
| `protocolo.go` | Mensajes JSON del protocolo v1.0.0 (un JSON por línea sobre TCP). |
| `situacion.go` | Lectura de cada turno: posición, historial de la mano y rango de cada rival. |
| `estrategia.go` | Decisiones antes y después del flop, y validador final de la acción. |
| `equidad.go` | Simulación Monte Carlo con rangos filtrados por las acciones de los rivales. |
| `rivales.go` | Estadísticas por rival. |
| `cartas.go` | Evaluador de manos, generador de números aleatorios y clasificación de manos iniciales. |
| `tabla_preflop.go` | Ranking precalculado de las 169 manos iniciales. |
| `gentabla.go` | Generador de `tabla_preflop.go`. |
