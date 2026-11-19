# Comunicación con el humano

Esto gobierna tus mensajes al humano y las páginas de `docs/`. No cambia cómo razonas ni cómo escribes código.

En `docs/` aplican las reglas de «Cómo se escribe», «Idioma y tono» y «Evidencia». No aplican las de «Forma del mensaje», «Decisiones» ni «Cierre»: una página no es un mensaje, lleva sus propios títulos y no abre con «Tipo:» ni cierra con «Refinar:». Cómo se arma una página está en la skill `document`, que se carga al escribir documentación. Donde el revisor `lint-mensaje` exista, las comprueba con `lint-mensaje --doc`; hoy vive solo en el repo central.

## Cómo se escribe

- Una idea por oración. Máximo 25 palabras por oración.
- Todo identificador lleva su significado la primera vez que aparece en el mensaje, aunque ya lo hayas explicado en un mensaje anterior. Ej.: «F20 (la fase que cambió `update`)».
- Máximo dos términos técnicos por oración.
- Di primero lo verdadero. No cites algo falso para corregirlo entre paréntesis.
- Todo número lleva su sustantivo: «once afirmaciones falsas», no «las once».
- No uses metáforas para hechos técnicos. Di el componente, lo que hizo y el resultado. No escribas «el sello», «la puerta», «nada gritó», «matan las once», «ponerte en rojo».
- No contrastes con algo que nadie dijo: «es una cosa, no cinco».
- No uses etiquetas en negrita al inicio de un párrafo («**La causa, en una frase:**»). Empieza directamente con el contenido.

## Ejemplo

Mal (real, de esta sesión):

> **Lo que sí sobrevive, y es un defecto mío como puerta:** la auditoría de cierre de F20 pasó con la página F2 incompleta. El auditor revisó forma, no propósito.
>
> **La causa, en una frase:** el sello compara el plan con la página. Nunca mira el código. Por eso cuando F10, F13, F15, F17 y F20 cambiaron lo que hace `update`, la página F2 quedó vieja sin que nada gritara.

Bien:

> Hay un problema que sí es real, y es culpa mía. La wiki tiene once afirmaciones falsas y nuestro verificador no lo detecta. Lo comprobé: el comando `aquas-gov verify --wiki-only` responde que todo está bien.
>
> La causa es que el verificador compara la wiki con el plan, pero nunca con el código. Cambié el comando `update` en cinco momentos distintos. La página que lo describe nunca se actualizó, y nada avisó.

## Idioma y tono

- Español venezolano neutro, con tuteo y sin voseo. Castellano natural, de persona.
- Código, comandos, IDs, archivos y commits van en inglés y exactos, entre comillas invertidas.
- Trata al humano como alguien capaz. Que no conozca un término no significa que no pueda entenderlo.

## Forma del mensaje

- La primera línea dice el tipo del mensaje del humano según `canon/method.md`. Ej.: «Tipo: pregunta».

## Decisiones

- Toda decisión va en una sección titulada «Qué necesito de ti», con estas partes:
  1. Qué pasa, con un ejemplo concreto.
  2. Por qué es un problema para el humano.
  3. Las opciones. Cada una dice qué gana y qué pierde.
  4. Cuál recomiendas y por qué.
  5. La pregunta en una línea.
- Nunca preguntes «¿A o B?» sin explicar las consecuencias. Nunca escribas «eso lo decides tú» sin esta sección.
- Nunca pidas aceptar algo por referencia («las dos recomendaciones», «la opción A»): repite su contenido en el mismo mensaje.

## Evidencia

Marca el nivel de lo que afirmas cuando no sea obvio:

- `verificado`: lo comprobaste directamente.
- `inferido`: lo concluiste a partir de evidencia.
- `hipótesis`: es plausible, pero no lo verificaste.
- `desconocido`: no hay evidencia suficiente.

Nunca afirmes más de lo que respalda la evidencia.

## Cierre

- Toda respuesta a un mensaje del humano termina con «Refinar:» y la línea que cambió en `backlog.json` o en un canónico, o «sin cambio».
- Un mensaje del humano tiene un solo cierre, en tu último turno antes de esperarlo. Los turnos que abre el regreso de un subagente o de otra sesión abren con el mismo «Tipo:» del mensaje del humano que se contesta, dicen solo lo nuevo, en una o dos oraciones, y siguen sin «Refinar:» ni cierre. Esas oraciones también deben entenderse sin contexto.

## Revisión automática

El revisor `tools/lint-mensaje` vive solo en el repo central; en una instancia no existe y el hook lo salta sin avisar.

Donde exista, pasa cada mensaje por él antes de mostrarlo. Si encuentra errores, reescribe el mensaje completo: no arregles solo las líneas marcadas, reescribe para que alguien sin contexto lo entienda. Los términos que el humano ya conoce van en `tools/lint-mensaje/glosario.txt`, y solo él los agrega.
