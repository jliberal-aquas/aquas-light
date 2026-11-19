# Método

Cómo se construye. El qué y el orden están en el backlog de `backlog.json`.

2. El backlog es liviano. Cada entrada de `backlog.json` lleva `id`, `type`, `parent`, `state`, `priority` y `spec`, y las features llevan además `capability`. El detalle de cada iniciativa, feature y slice vive en `work/specs/<ID>.json`. Un slice dice qué hace y qué archivos toca. Dice su prueba funcional (lo que el humano observa) y su prueba técnica (nombre del test, escrito y en rojo antes del código). Sin código: «Prueba técnica: no aplica (sin código)». Una feature tiene una sola línea «Aceptación:» y cada slice la acerca; una segunda aceptación es otra feature.
3. Los IDs usan contadores globales por tipo: `I` iniciativa, `F` feature, `S` slice, `B` bug, `N` necesidad, `C` capacidad. Los estados son `to_refine`, `proposed`, `ready`, `in_progress` y `done`. Solo un slice puede estar en `in_progress`.
4. Al terminar un slice, pon su `state` en `done` en `backlog.json` y haz commit en la rama principal del repo, sin renombrarla. Al cerrar una feature, muda su entrada a `archive/<iniciativa>.json` con `closed_at` y `evidence`. Sin ramas ni PR por ahora.
5. La wiki va por capacidad, no por feature. Cada capacidad de la lista `capabilities` de `backlog.json` tiene su página en `docs/<lado>/<ID>-<nombre>.md`, donde `<lado>` es `governance` o `product`. Al cerrar una feature, actualiza la página de cada capacidad que tocó. Si la feature creó una capacidad, declárala y escribe su página en la misma tanda, porque `verify` rechaza el commit de una capacidad sin página. El humano valida la feature leyendo esas páginas. Cómo se arma una página está en la skill `document`.
6. Cada rol trabaja con el modelo y el esfuerzo de `canon/roles.json`. El arquitecto revisa el plan de cada feature justo antes de empezarla, y escribe los casos destructivos. El auditor audita la feature al cerrarla; nunca revisa slices. Ambos corren como subagentes de contexto limpio dentro de la sesión del orquestador, nunca en una sesión aparte. Cada rol se manda con su tipo de subagente, el de su archivo de `.claude/agents/`, y la llamada no lleva modelo ni esfuerzo escritos: los fija ese archivo. En el repo central un gancho niega la llamada que los traiga; en una instancia todavía no existe ese gancho, así que ahí la regla es palabra tuya. Lo que traiga fuera de lo pedido va a «Fuera de pedido», sin otra ronda. El humano aprueba la iniciativa I1 ahora y cada feature al empezarla.
8. Un control nuevo nace solo de una falla que el humano vio. Anótala en `work/defects.md` con fecha, falla, control y prueba. No diseñes controles por iniciativa propia.
11. Un slice con código pasa a `done` solo después del tester: la prueba técnica da verde, y en rojo sin el código del slice. Los slices sin código no necesitan informe del tester.
12. Al cerrar una feature, di en una línea «Hay N candidatos para retrospectiva» o «Sin candidatos para retrospectiva». El auditor anota los candidatos en `work/retro.json` al cerrar la feature, y N es cuántos hay ahí. Solo el humano dispara la retrospectiva, con `/retrospective`, y la corre el auditor. Ningún control se aplica sin ese comando.
13. Todo commit lleva la línea `Docs: revisé=<ruta de docs/ o nada>; falsa=<sí|no>; promesa=<sí|no>`. El hook `commit-msg` la exige y la lee de `git log`; no se copia a ningún otro archivo. Su contenido es tu palabra y queda registrado con fecha. `--no-verify` no se usa: salta todos los hooks.
14. Una sesión dura una feature. Al archivarla, cierra la sesión con `/clear` y arranca la siguiente leyendo `backlog.json` y `work/memory.json`. El motivo es el costo: cada respuesta del modelo vuelve a leer la conversación entera, así que el gasto de un turno es el tamaño del contexto, y una sesión larga lo paga en cada llamada.
15. Un slice `done` nunca se modifica. Un cambio de alcance crea un slice nuevo con `supersedes`. Si el cambio toca una feature archivada, nace una feature nueva que la referencia.
16. Un bug es una entrada propia (`B`) con padre en un slice `done`. Nace con una prueba roja que reproduce la falla. El slice no se reabre.
17. El barrido corre al cerrar un slice: primero los bugs de la feature en curso, por prioridad. Las necesidades en `to_refine` se refinan al cerrar la feature, en una sesión nueva, salvo urgencia del humano.
18. La memoria caliente es `work/memory.json`, con los campos `slice`, `step`, `done`, `next`, `files`, `blockers` y `updated_at`. `step` vale `plan`, `red`, `green`, `tester`, `docs` o `commit`. El gancho Stop bloquea si un archivo cambió después de `updated_at`. Al cerrar el slice se archiva en `archive/memory/<ID>.json`.
19. Cada necesidad tiene su registro en `work/requests/<N>.json`. Al llegar el mensaje se crean `date`, `turn` y `type`. Al refinar se llenan `scenario`, `rounds` (pregunta, recomendación y respuesta), `result`, `adr` y `glossary`. El texto crudo lo guarda el gancho `UserPromptSubmit` en `work/turns.jsonl`, y el registro apunta a ese turno.
20. El gancho Stop bloquea el turno si falla alguno de estos ocho puntos:
    1. La primera línea dice «Tipo:».
    2. La última línea dice «Refinar:».
    3. Si cambió código, corrieron test y lint de su lado con código 0.
    4. La memoria está al día.
    5. No hay sellos vencidos.
    6. Hay como máximo un slice en `in_progress`.
    7. Si hay un slice con código en `done`, existe su informe del tester.
    8. Se agregó la línea de telemetría.
21. Cuando se salta un gancho, el caso se anota en `work/gate-bypass.jsonl` con `id`, `date`, `hook`, `reason` y `status`. Se cierra con `aquas bypass close <id> <razón>`. Un caso abierto bloquea empezar y cerrar slices. El auditor lee las razones al cerrar la feature.
22. Un sello ata un documento a lo que describe en `work/seals.json`. Guarda la huella SHA-256 del documento y de cada archivo o especificación que cubre. Si algo cambia, el documento queda vencido y el commit se bloquea. La cadena de cobertura es bug, slice, feature, iniciativa y capacidad.
23. El resello vale solo si el documento cambió o si trae `unchanged_reason`, de máximo 300 caracteres, que explica por qué sigue siendo cierto. El auditor lee esas razones al cerrar la feature.
24. El repo tiene dos lados: gobierno y producto. La documentación del gobierno va en `docs/governance/` y la del producto en `docs/product/`. `README.md` es la entrada única. Cada cambio se valida con las herramientas del lado que toca, nunca con las del otro.
25. En una instancia no se edita `tools/`; el verificador rechaza el commit que lo toque, salvo una actualización de la skill. Las mejoras van a `work/retro.json` como candidatas para el paquete. El canon en Markdown sí se edita.
26. La instalación solo ocurre en una carpeta vacía: que no existe, o que no tiene ningún archivo, ni siquiera `.git`. La skill corre fuera de ella, la crea si falta y corre `git init`. Si trae algo, no crea nada. Al terminar, el humano abre Claude Code en la carpeta nueva.

## Refinamiento
Antes de actuar, clasifica cada mensaje del humano. Si mezcla tipos, sepáralo y trata cada parte por el suyo.
- **Comentario:** no pide nada. Recíbelo; no cambia el trabajo.
- **Pregunta:** busca la respuesta y dala, sin abrir trabajo.
- **Respuesta:** el input que pediste, incluido el visto bueno para seguir con lo ya acordado. Úsalo en el slice en curso.
- **Necesidad:** pide construir o cambiar algo. Es feature nueva o cambio de alcance (modifica algo ya acordado).
- **Error:** algo no se comporta como se esperaba.
- **Orden:** pide ejecutar algo puntual que no cambia el plan ni el código (un commit, un comando, borrar un temporal). Hazla directo, también con un slice en curso. Si pide cambiar código o el plan, es necesidad.

1. Abre el refinamiento de cada necesidad con contexto: qué pidió el humano (sus palabras y fecha), qué entendiste contado como escenario (quién, dónde, cómo arranca, qué hace y qué observa al final) y qué falta preguntar. Cada pregunta se apoya en ese escenario.
2. Sin slice en curso, interroga cada necesidad y cada error en rondas con la skill `grill-with-docs`. La skill está copiada en `.claude/skills/` y fijada a su commit de origen. Si no está, usa su formato: preguntas numeradas, cada una con tu recomendación, hasta que no quede duda. Los comentarios, las preguntas, las respuestas y las órdenes no se interrogan. Recién ahí escríbela como feature y slices en `work/specs/` y en `backlog.json`, y el humano la aprueba.
3. Con un slice en curso, aparca toda necesidad o error en `backlog.json` como entrada `to_refine`, con su campo `source` que apunta a `work/requests/<N>.json`. No la trabajes. No existe `work/inbox.json`.
