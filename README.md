# Bot F&D — PokerFight

## Estilo: tight-aggressive (selectivo y agresivo) con explotación de rivales

- **Selectivo:** entra en pocas manos y solo con manos que tienen valor según su posición en la mesa.
- **Agresivo:** cuando entra, casi siempre lo hace subiendo. Apuesta fuerte sus manos buenas para cobrarles a los rivales que pagan de más, en lugar de pagar o pasar.
- **Explotador:** aprende cómo juega cada rival y ajusta contra él. Les apuesta más grande a los que pagan todo, nunca les farolea y se retira ante los que solo apuestan con manos fuertes.
- **Disciplinado con el riesgo:** farolea poco y con límites estrictos. Antes de arriesgar una parte grande de sus fichas exige ventaja adicional. En la mesa final relaja esa disciplina y juega más cerca del valor esperado puro.

## Mecánica

### 1. Antes del flop

- **Ranking de las 169 manos iniciales.** Cada mano tiene un percentil (0 = AA, 1 = la peor), calculado por simulación contra 3 rivales y ajustado por jugabilidad: las manos del mismo palo y conectadas suben; las de kicker débil bajan.
- **Si nadie subió**, abre con un rango que depende de cuántos jugadores faltan por hablar. Abre con el 14 % mejor de las manos si quedan 5 por hablar, con el 42 % desde el botón y con el 70 % mano a mano. El rango se cierra si ya hay rivales que solo igualaron la ciega. Sube 2,5 ciegas grandes, más una por cada uno de esos rivales, y aún más si la mesa paga demasiado.
- **Si alguien subió**, deduce qué manos puede tener ese rival según lo que hizo (igualar, subir, resubir o ir all-in) y según qué tan suelto juega. Luego simula su probabilidad de ganar contra ese rango y la compara con el precio de pagar:
  - **Resube** si va claramente por delante.
  - **Iguala** si la probabilidad cubre el precio más un margen. El margen crece si está fuera de posición, si ya hubo varias subidas o si quedan rivales por hablar.
  - **Se retira** en cualquier otro caso.
- **Con stack corto** (14 ciegas grandes o menos), juega all-in o se retira, sin términos medios.

### 2. Después del flop

- **Simulación Monte Carlo** (hasta 20 000 manos simuladas o 400 ms). En cada simulación reparte cartas a los rivales y completa el tablero. Las cartas de cada rival se filtran por sus acciones en la mano: quien apuesta o sube rara vez tiene una mano débil, salvo que su historial muestre que farolea.
- **Si nadie apostó**, compara el valor esperado de pasar con el de apostar un 33 %, 60 % o 90 % del pozo. Contra rivales que pagan todo y con una mano muy fuerte, también considera apostar un 140 % del pozo. El cálculo tiene en cuenta qué tan seguido se retira cada rival ante una apuesta, y el bot elige la opción con mayor valor esperado.
- **Faroles** solo cuando se cumplen todas estas condiciones:
  - Hay uno o dos rivales en la mano, y ninguno paga todo.
  - La apuesta no supera el 60 % del pozo y, en la clasificatoria, el 15 % de sus fichas.
  - El bot fue el último en subir antes del flop, o está en posición y nadie lo ha presionado.
- **Si le apostaron**, compara su probabilidad de ganar con el precio:
  - **Sube** con manos muy fuertes. Si la subida compromete la mitad de sus fichas, va all-in.
  - **Iguala** si la probabilidad cubre el precio más el margen de riesgo.
  - **Iguala barato con proyectos** de color o de escalera antes del river.
  - **Se retira** en cualquier otro caso.

### 3. Modelo de cada rival

Por cada rival guarda estas estadísticas, que se mantienen de una ronda a otra:

- Con qué frecuencia entra en el pozo.
- Con qué frecuencia sube antes del flop.
- Con qué frecuencia va all-in antes del flop.
- Qué tan agresivo es después del flop.
- Con qué frecuencia se retira cuando le apuestan.

Cada estadística parte de un valor inicial razonable, para no sacar conclusiones con pocas manos observadas. Con ellas el bot ajusta:

- El rango de manos que le atribuye al rival.
- La probabilidad de que esté faroleando.
- El tamaño de sus apuestas por valor.
- Si vale la pena farolearle.

### 4. Control de riesgo

Antes de arriesgar fichas, exige una probabilidad de ganar adicional proporcional a la parte de su stack en juego: hasta 9 puntos porcentuales en la clasificatoria y 2 en la final. Con stack corto ese margen se reduce. Además, cada acción pasa por un validador final que garantiza que sea legal y que se envíe dentro del tiempo límite.
