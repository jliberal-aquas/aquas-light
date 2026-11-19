# Delegar

Todo encargo entre agentes va en una sola llamada, con la forma: un `context` compartido y una lista `tasks`. Todo informe de vuelta es un JSON con el esquema del informe, que el arnés debe validar. Los nombres de los campos van en inglés y el contenido en español.

## Esquema del encargo

| field | techo | qué lleva |
|---|---|---|
| `context` | 1.200 caracteres | lo que los ayudantes necesitan saber y no saben |
| `tasks[].agent` | un `id` de `canon/roles.json` distinto de `orchestrator` | a quién va |
| `tasks[].item` | un ID de slice o de feature | el trabajo al que pertenece el encargo |
| `tasks[].task` | 2.000 caracteres | qué hacer, qué archivos, qué no hacer, y una oración que empieza con «Terminas cuando» |
| `tasks[].solutionSpace` | 300 caracteres | qué tan abierto es el encargo |

Cada tarea puede traer además los campos opcionales que el arnés admita: `name`, `outputSchema`, `schemaMode`, `tools` e `isolated`. No puede traer `model` ni `effort`, porque el archivo del rol fija el modelo y el esfuerzo. Cualquier otro campo se rechaza.

## Esquema del informe

Está en `schemas/report.schema.json`. Siete campos obligatorios y uno opcional, sin campos sobrantes:

| field | techo | qué lleva |
|---|---|---|
| `item` | un ID de slice o de feature | el trabajo al que responde el informe; es el `item` del encargo |
| `conclusion` | 2.000 caracteres | la respuesta al objetivo, primero |
| `findings` | 20 objetos | cada uno con `what`, `where`, `why`, `certainty` y `severity` |
| `evidence` | 20 objetos | cada uno con `file`, `line` (texto, admite «42-57») y `quote` de hasta 300 caracteres |
| `output` | 2.000 caracteres, opcional | la salida literal de un comando, si se pidió |
| `pending` | lista | lo que quedó sin hacer; puede ir vacía |
| `certainty` | `verified`, `inferred`, `hypothesis` o `unknown` | cuánto respalda la evidencia a la conclusión |
| `result` | `pass`, `fail` o `n/a` | el resultado del trabajo |

`severity` vale `blocker`, `major` o `minor`.

## Dónde queda el informe

El gancho que recibe el informe escribe `work/reports/<item>-<agent_type>.json`. `<item>` es el `item` del encargo y `<agent_type>` es el tipo de subagente que respondió. Ejemplo: `work/reports/S2-implementer.json`.
