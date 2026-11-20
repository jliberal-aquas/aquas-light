# C2. Verificar

Esta página describe lo que hace el código hoy, no lo que promete el plan. Cada afirmación cita el archivo y la función donde está. Cita la prueba que la cubre cuando existe; si no existe, lo dice con «Sin prueba».

Los niveles de evidencia son los de `canon/communication.md`. Aquí todo está `verificado` contra el código el 2026-11-20, salvo lo marcado como `inferido`, `hipótesis` o `desconocido`. Las corridas de integración continua las verificó el orquestador. Esa fecha también incluye una corrida real del binario compilado desde `tools/`, que dio código 0 en la raíz de este repo. Las rutas de código (`cmd/` e `internal/`) son relativas a `tools/`. Las demás rutas (`canon/`, `schemas/`, `templates/`, `work/`, `docs/`) son relativas a la raíz del repo.

## Para qué sirve C2

C2 es la capacidad de comprobar que los archivos JSON que gobiernan el trabajo tienen la forma y las relaciones que el canon exige. Se ejerce con el comando `aquas verify`.

F2 es la feature (el entregable del plan) que construyó C2. Tiene cuatro slices, que son sus pasos de entrega.

- S9 creó los tipos de cada archivo, el decodificador estricto y el comando.
- S10 agregó las reglas del backlog.
- S11 agregó las reglas de capacidades y sus páginas.
- S12 hizo que `aquas install` exija un resultado verde.

El backlog es la lista de trabajo en `backlog.json`. Una capacidad es algo que el repo sabe hacer y que tiene su propia página, como esta. Una spec es el archivo `work/specs/<ID>.json` que describe un elemento del backlog.

El verificador no corrige nada. Lee archivos, junta las violaciones y las imprime.

## Cómo se corre

El comando es `aquas verify [--staged]`. Desde un clon preparado con `setup`, el binario está en `bin/aquas`. Desde el paquete se corre con `go run ./cmd/aquas verify`, con el directorio actual dentro de `tools/` (`cmd/aquas/main.go`, función `runVerify`).

### Cómo encuentra la raíz

El comando sube desde el directorio actual hasta la primera carpeta que tiene un archivo regular `backlog.json`. Esa carpeta es la raíz (`internal/verify/verify.go`, función `FindRoot`). Por eso da el mismo resultado corrido desde `tools/internal/` que desde la raíz.

Si no encuentra la raíz, imprime en `stderr` que no se encontró `backlog.json` y sale con código 3. Prueba: `TestFindRoot`, `TestVerifyWithoutBacklogIsCode3` y `TestVerifyGreenAndFromSubfolder`.

### Qué lee cada modo

Sin opciones, el comando lee el disco, es decir, el árbol de trabajo (`Disk`). No usa git.

Con `--staged`, lee el índice de git, que es lo que `git add` dejó listo para el próximo commit (`Index`). Los archivos editados y sin agregar no cuentan. Los archivos agregados y después cambiados en disco cuentan con el contenido agregado.

Ambos modos usan la interfaz `Source`. Las reglas no saben de dónde vienen los archivos.

El modo `--staged` corre estos comandos de git dentro de la raíz: `rev-parse --show-prefix`, `ls-files -z -s` y `cat-file blob`. Prueba: `TestIndexSource`, `TestVerifyStagedReadsIndex` y `TestVerifyBacklogRulesStagedReadsIndexNotDisk`.

### `GIT_INDEX_FILE` y `GIT_DIR`

`--staged` respeta `GIT_INDEX_FILE`, la variable que apunta a un índice alterno. Si trae una ruta relativa, la vuelve absoluta respecto del directorio actual del proceso (`gitEnv`). Prueba: `TestIndexSourceHonorsGitIndexFile`.

`--staged` ignora `GIT_DIR` y `GIT_WORK_TREE` heredados. Así, un gancho de git que corre dentro del entorno de otro repo igual revisa la raíz encontrada. Prueba: `TestVerifyBacklogRulesStagedIgnoresInheritedGitDir`.

`--staged` falla con código 3 en dos casos. Primero, la raíz no tiene `.git`. Segundo, la raíz está dentro de otro repositorio y no tiene su propio `.git`. En ambos el mensaje dice «no es un repositorio git». Si `git` no está en el `PATH`, el mensaje lo dice. Prueba: `TestVerifyBacklogRulesStagedWithoutGit` y `TestIndexOutsideRepoFails`. El mensaje de `git` ausente no tiene prueba.

### Argumentos

La única opción es `--staged`, y se puede repetir sin efecto. Cualquier otra opción que empiece con `-` imprime «opción desconocida» y el uso. Cualquier argumento sin guion imprime «sobra el argumento» y el uso. Los dos casos salen con código 2. Prueba: `TestVerifyBadArgsIsCode2`.

Hoy no hay ganchos de git en este repo: la carpeta `.githooks/` no existe. Nada corre `aquas verify` de forma automática. La integración continua lo ejerce de forma indirecta: `TestRepoFilesDecode` corre el verificador sobre los archivos reales del repo y falla con cualquier violación (`.github/workflows/ci.yml` corre `go test -race ./...`).

## Mensajes y códigos de salida

Cada violación sale en `stdout`, una por línea, con este formato (`Violation.String`):

`<archivo>: <posición>: <mensaje>`

El archivo es la ruta relativa a la raíz, escrita con `/`. La posición es la ruta dentro del JSON, por ejemplo `items[2]`, `items[2].priority`, `next_ids.S` o `capabilities[0].page`. Para un problema del archivo entero, la posición es `raíz`.

Ejemplo real de una corrida con un campo inventado: `backlog.json: items[3]: campo sobrante «extra»`.

El comando ordena las violaciones por archivo y, dentro de un archivo, por la posición del texto que las causó. Dos corridas seguidas dan la misma salida. Prueba: `TestDecodeRejectsUnknownFields`, subprueba «tres fallas en un archivo, orden y determinismo».

Los mensajes están en español. Ningún texto en inglés de la biblioteca de Go llega al humano. Para un JSON mal formado, el decodificador escribe su propio mensaje con el número de línea. Prueba: `TestDecodeRejectsUnknownFields`, subpruebas «JSON mal formado» y «tipos, null y campo ausente».

Códigos de salida (`runVerify`):

- 0: no hay violaciones. Imprime «Verificación en verde: no hay violaciones.» en `stdout`.
- 1: hay al menos una violación. Las imprime todas en `stdout`.
- 2: los argumentos son inválidos. El mensaje sale en `stderr`.
- 3: no se pudo verificar. Los casos son: no se leyó el directorio actual, no hay raíz, falló la lectura del índice de git o falló la lectura de un archivo. El mensaje empieza con «no se pudo verificar» y sale en `stderr`.

El código 3 no es un veredicto sobre los archivos. Dice que la comprobación no corrió. Prueba: `TestVerifyReportsViolations` (código 1), `TestVerifyBadArgsIsCode2` (código 2), `TestVerifyWithoutBacklogIsCode3` (código 3) y `TestVerifyGreenAndFromSubfolder` (código 0).

## Qué archivos decodifica

El verificador decodifica estos archivos (`internal/verify/verify.go`, función `Run`):

| Archivo | Tipo de Go | Obligatorio |
| --- | --- | --- |
| `backlog.json` | `Backlog` | Sí |
| `canon/config.json` | `Config` | Sí |
| `archive/*.json` | `Archive` | No |
| `work/specs/I<N>.json` | `SpecInitiative` | No |
| `work/specs/F<N>.json` | `SpecFeature` | No |
| `work/specs/S<N>.json` | `SpecSlice` | No |
| `work/specs/B<N>.json` | `SpecBug` | No |
| `work/requests/*.json` | `Request` | No |

Si falta un archivo obligatorio, el mensaje es `<archivo>: raíz: falta el archivo`. Una carpeta ausente no es una violación: se trata como una lista vacía. Solo cuentan los `.json` directamente dentro de cada carpeta, sin subcarpetas.

El tipo de una spec sale de la primera letra del nombre del archivo, y nada más. Un nombre que no empieza con I, F, S ni B da la violación «nombre de spec desconocido». Un nombre como `Foo.json` se trata como spec de feature, porque empieza con F. Prueba del nombre desconocido: `TestDecodeRejectsUnknownFields`, subprueba «role y archivo de spec desconocido». El caso `Foo.json` no tiene prueba.

El verificador no decodifica otros JSON, como `canon/manifest.json`, `work/host.json` o `work/seals.json`.

Los tipos nacen de los archivos que existían al cerrar S9. Un `technical_test` puede ser un texto o un objeto con `name` y `red_before_code`. En una spec de bug es opcional. Las solicitudes admiten los campos de N1 y N2 (las dos solicitudes que ya había en `work/requests/`), y `turn` es opcional. El campo `role` de `canon/config.json` vale `pack` o `instance` (`internal/schema/types.go`). Prueba: `TestDecodeRejectsUnknownFields`, subpruebas «formas válidas de technical_test», «solicitud con turn y campos nulos» y «role y archivo de spec desconocido». Prueba contra los archivos reales: `TestRepoFilesDecode`.

## Qué rechaza el decodificador estricto

El decodificador recorre las claves del JSON por su cuenta, sin el emparejamiento de `encoding/json` (`internal/schema/decode.go`, función `Decode`). Los tipos de Go llevan etiquetas `json` y `aq` que dicen qué se permite. Rechaza:

- Campos sobrantes. Si solo cambian las mayúsculas, el mensaje dice cómo se llama el campo. Las claves distinguen mayúsculas.
- Claves repetidas en un objeto, también dentro de valores libres como `decisions`.
- Campos obligatorios ausentes.
- `null` en un campo que no lo admite. En `decisions` de una solicitud, `null` también se rechaza.
- Valores de otro tipo, como un número donde va un texto.
- Valores fuera de una lista cerrada, como `state`, `type`, `role` o `platforms`. El mensaje lista los valores válidos.
- Enteros escritos con decimales o exponente, como `1.5` o `1e2`, y enteros por debajo del mínimo del campo.
- Un BOM de UTF-8 al inicio.
- Un archivo vacío.
- Contenido después del final del valor. Los espacios y saltos de línea finales sí se permiten.
- JSON mal formado o cortado antes de cerrar.

Los finales CRLF se aceptan. Un error de sintaxis detiene la decodificación de ese archivo, pero conserva las violaciones ya encontradas en él.

Prueba: `TestDecodeRejectsUnknownFields`, con una subprueba por cada caso de la lista, salvo estos tres. El caso de `null` en `decisions` no tiene prueba. El de claves repetidas dentro de `decisions` tampoco. El de cambio de mayúsculas se prueba en la subprueba «mayúsculas en las claves».

## Los JSON Schema publicados

La carpeta `schemas/` publica un JSON Schema (borrador 2020-12) por cada tipo, con el nombre `<nombre>.schema.json`: `backlog`, `archive`, `spec-initiative`, `spec-feature`, `spec-slice`, `spec-bug`, `request` y `config`.

Se generan desde los mismos tipos de Go y las mismas etiquetas, con código propio (`internal/schema/generate.go`, función `Generate`). La lista está en `Registry`, en `internal/schema/types.go`. La salida es determinista y usa finales LF.

El verificador no lee `schemas/` al correr. Los esquemas sirven a otras herramientas y a los editores.

Para regenerarlos, corre esto desde `tools/`:

`go run ./internal/schema/gen ../schemas`

El generador sobrescribe los ocho archivos. No borra archivos de `schemas/` que ya no estén en `Registry`. Una corrida en una carpeta temporal produjo archivos idénticos a los de `schemas/` el 2026-11-20.

Si cambias un tipo de Go y no regeneras, `TestSchemasMatchTypes` falla y nombra el archivo desactualizado. Prueba: `TestSchemasMatchTypes`, `TestSchemaStaleIsDetectedByName`, `TestGenerateChangesWithTypes` y `TestGeneratedSchemaShape`.

## Las reglas del backlog

Las reglas del backlog son ocho. Se aplican a `backlog.json` y a `archive/*.json` juntos (`internal/verify/backlog.go`, función `backlogRules`). Corren solo si `backlog.json` existe y decodificó sin violaciones.

Si un archivo de `archive/` no decodifica, sus elementos quedan fuera de las reglas. Entonces las reglas que buscan un ID que falta (padre, `supersedes`) no lo reportan, porque la ausencia no prueba nada. La violación del archivo roto sí sale. Prueba: `TestVerifyBacklogRules`, subprueba «archivo archivado ilegible».

1. Formato del ID. Debe cumplir `^[IFSBNC][1-9][0-9]*$`, y su letra debe corresponder al tipo: I iniciativa, F feature, S slice, B bug, N necesidad. La letra C es solo de capacidades y se rechaza en un elemento. Los IDs de `capabilities` deben empezar con C. Un ID con un número demasiado grande da un mensaje, no un pánico. Prueba: subpruebas «IDs mal formados» y «letra distinta del tipo».
2. Un ID aparece una sola vez entre `backlog.json` y `archive/*.json`. El mensaje nombra los dos lugares. Prueba: «ID repetido».
3. Cada contador de `next_ids` es mayor que todo ID de su letra en `backlog.json`, en `capabilities` y en `archive/*.json`. Un contador igual al mayor ID se rechaza. Prueba: «contador atrasado respecto de un ID del backlog», «contador atrasado respecto de un ID archivado», «contador de capacidades» y «contador igual al mayor ID».
4. Padres. Una iniciativa tiene padre `null`. Una feature tiene una iniciativa. Un slice tiene una feature. Un bug tiene un slice en estado `done`. Una necesidad tiene `null` o cualquier ID existente. El padre puede estar archivado. Prueba: «padres inválidos» y «padre archivado».
5. A lo más un slice o bug está en `in_progress`. Una feature o iniciativa en `in_progress` no cuenta. Prueba: «un slice o bug en curso».
6. Cada necesidad en `to_refine` tiene `source`, y apunta a un archivo existente de `work/requests/` con la forma `work/requests/<archivo>.json`, sin `..`. Prueba: «necesidades por refinar».
7. `supersedes` solo lo lleva un slice, y apunta a otro slice en estado `done`, que puede estar archivado. No puede apuntarse a sí mismo. Prueba: «supersedes inválido», «supersedes hacia un slice done archivado» y «en un elemento que no es slice».
8. Huella de las specs cerradas. Todo slice o bug en `done` lleva `spec_sha256`, que son 64 caracteres hexadecimales en minúscula. Su campo `spec` apunta a un archivo de `work/specs/`. La huella coincide con la del contenido actual de ese archivo, calculada con `hashing.Sum`, que normaliza los finales a LF. Un elemento que no está en `done` no lleva `spec_sha256`, y las features tampoco. Prueba: «spec alterada con un espacio», «spec con finales CRLF», «huella inválida o fuera de lugar» y «una feature no lleva huella».

En modo `--staged`, la regla 8 lee la spec desde el índice. Prueba: `TestVerifyBacklogRulesStagedReadsIndexNotDisk`.

## Las reglas de capacidades

Las reglas de capacidades son tres. Corren como parte de las reglas del backlog (`internal/verify/capabilities.go`, función `capabilityRules`).

1. Toda feature, en `backlog.json` o archivada, tiene `capability`, y apunta a una capacidad declarada en `capabilities`. Prueba: `TestVerifyCapabilityPages`, subpruebas «features con capacidad inválida» y «feature archivada con capacidad inexistente».
2. Ningún ID de capacidad ni `page` se repite. Prueba: «ID y página repetidos».
3. Toda capacidad con al menos una feature archivada tiene su página. Una capacidad sin feature archivada no la exige. Prueba: «capacidad sin features archivadas no exige página» y «plantilla vacía».

Con esto, la regla 14 de `canon/constitution.md` (cada capacidad tiene su página) queda comprobada solo al archivar la primera feature de la capacidad. Mientras F2 siga abierta, `docs/governance/C2-verify.md` no es obligatoria para el verificador.

La página de una capacidad es válida solo si cumple todo esto (`pageProblem`):

- La ruta se escribe con `/`, sin barras invertidas, y no es absoluta ni lleva `:`.
- La ruta está normalizada y no sale del repo: sin `.` ni `..`.
- La ruta tiene la forma `docs/governance/<archivo>` o `docs/product/<archivo>`, directamente en esa carpeta.
- El archivo se llama `<ID>-<nombre>.md`, con el ID de la misma capacidad.
- Cada tramo existe con las mayúsculas exactas. El verificador lo comprueba listando cada carpeta y comparando nombres, no con `os.Stat`.
- Ningún tramo es un enlace simbólico.
- El destino es un archivo normal, no una carpeta.
- El archivo no está vacío. El verificador no revisa su contenido.

Antes de leer el archivo, el verificador comprueba la ruta como texto y recorre las carpetas por listados. Así nunca abre un archivo fuera del repo. Prueba, en `TestVerifyCapabilityPages`: «rutas que escapan o no son relativas», «barras invertidas aunque el archivo exista» y «fuera de docs/governance y docs/product». También «docs/product es válido», «página de otra capacidad», «mayúsculas distintas en disco» y «enlace simbólico a un archivo de fuera».

## Cómo `aquas install` exige verde

`aquas install` llama al verificador dentro del mismo proceso, en modo disco, sobre la carpeta destino. Lo llama una sola vez, después de escribir `work/host.json` y antes del commit de instalación (`internal/install/install.go`, funciones `Run` y `runVerify`). En ese momento `bin/aquas` y `work/host.json` ya existen, y `HEAD` todavía no. Prueba: `TestInstallRunsVerify`, subprueba «el verificador corre antes del commit y después de bin y host» y «una sola llamada con la carpeta destino como raíz».

Lo que se verifica es la instancia recién copiada: su `backlog.json` sale de `templates/backlog.json`. El `backlog.json` de la raíz del paquete no cuenta. Prueba: «el backlog.json de la raíz del paquete no cuenta».

Si el verificador devuelve violaciones, `install` falla con el mensaje «lo instalado no pasa la verificación:» seguido de las violaciones, una por línea. Si el verificador no pudo correr, falla con «no se pudo verificar lo instalado:» y el error. En los dos casos `install` revierte todo, igual que ante cualquier otro fallo después del preflight, y sale con código 1. El mensaje completo empieza con «no se puede instalar (se revirtió todo lo creado)» (`cmd/aquas/main.go`, función `runInstall`).

Tras el rollback, la carpeta destino no existe si la creó la instalación, y queda con 0 entradas si ya existía. Prueba, en `TestInstallRunsVerify`: «plantilla rota en una ruta que no existe», «plantilla rota en una carpeta vacía existente» y «error de ejecución del verificador». Otra prueba: `TestIntegrationVerify`, fila «plantilla rota no deja nada».

Cuando el verificador da verde, `install` imprime primero la línea «Verificación en verde: no hay violaciones.» y después su mensaje de éxito. La línea verde sale una sola vez. Prueba: `TestIntegrationVerify`, subprueba «la salida muestra la línea verde una sola vez».

`Options.Verify` permite inyectar otro verificador en las pruebas. Con valor nulo, se usa el real.

`setup` y `update` no llaman al verificador. Prueba para `setup`: `TestInstallRunsVerify`, subprueba «setup no llama al verificador». Para `update`, `internal/update` no importa el paquete `verify`, lo que se comprueba leyendo el código. `update` no tiene prueba propia de esto. Tras un `update`, corre `aquas verify` a mano.

## Plataformas

El código contempla Linux, macOS y Windows, igual que `work/specs/F2.json`.

- Los mensajes y las rutas del verificador usan `/` en los tres sistemas.
- Una página con barras invertidas se rechaza en los tres sistemas.
- Las mayúsculas exactas se comprueban por listado en los tres sistemas, también donde el sistema de archivos no distingue mayúsculas.
- La prueba del enlace simbólico se salta con `t.Skip` solo si `os.Symlink` falla, algo posible en Windows sin permisos.
- El modo `--staged` necesita `git` en el `PATH`. El modo disco no lo usa.

La integración continua corre `go test -race ./...` en `ubuntu-latest`, `macos-latest` y `windows-latest` (`.github/workflows/ci.yml`). Las corridas 44536748393 (S9), 44557168312 (S10), 44776565561 (S11) y 44778308410 (S12) de GitHub Actions terminaron con `success` en los tres sistemas. El orquestador lo comprobó con `gh` el 2026-11-20.

## Limitaciones conocidas

1. S9 no valida techos de longitud. Un `name`, un `what` o un `raw_message` pueden estar vacíos o ser larguísimos y pasan. Cada techo se agregará en el slice que traiga un valor con origen. Lo decidió la revisión del arquitecto de F2, aprobada por el humano.
2. `priority` admite 0, porque su mínimo es 0. Tampoco se comprueba que las prioridades sean distintas. Sin prueba.
3. Con `--staged`, una página que existe en disco y no se agregó con `git add` da código 1 y nombra la capacidad. Pero el motivo puede decir «no existe la carpeta» y no «el archivo no existe». Ocurre cuando ningún otro archivo de esa carpeta está en el índice. En una corrida real con `docs/governance/` sin archivos agregados, el mensaje fue `no existe la carpeta "governance"`. La prueba «staged: la página está en disco y no en el índice» comprueba el código 1 y el nombre de la capacidad, no la redacción del motivo.
4. Con `--staged`, un archivo `.json` agregado como enlace simbólico se lista y se lee como si fuera un archivo normal. En modo disco, `List` omite los enlaces simbólicos. Sin prueba.
5. El verificador no cruza el contenido de las specs con el backlog. No comprueba que el `id` de una spec coincida con el nombre de su archivo. No comprueba que `parent` o `capability` coincidan con el backlog. Tampoco comprueba que los archivos de `files` existan ni que `slices` liste los slices reales. No valida formatos de fecha ni el contenido de `evidence`.
6. Un nombre de spec como `Foo.json` se decodifica con el tipo de su primera letra, aunque no siga el formato `F<N>.json`.
7. De una página de capacidad solo se comprueba que exista y no esté vacía. Que describa bien la capacidad lo revisa el humano o el auditor, no el verificador.
8. Nada corre `aquas verify` de forma automática al hacer commit, porque no hay ganchos en este repo. Quien edita los JSON debe correrlo.
9. `update` no verifica. Si un `update` cambia `canon/` o `tools/`, el resultado se comprueba corriendo `aquas verify` después.

## Qué prueba cubre cada afirmación

Todas las pruebas están en `tools/` y corren con `go test ./...`. Las subpruebas se nombran con el texto de su `t.Run`.

- Cómo encuentra la raíz: `TestFindRoot` en `internal/verify/verify_test.go`; `TestVerifyGreenAndFromSubfolder` y `TestVerifyWithoutBacklogIsCode3` en `cmd/aquas/main_test.go`.
- Origen disco y origen índice: `TestDiskSource`, `TestIndexSource`, `TestIndexSourceHonorsGitIndexFile` y `TestIndexOutsideRepoFails` en `internal/verify/verify_test.go`.
- `--staged` lee el índice, ignora `GIT_DIR` y falla sin repo propio: `TestVerifyBacklogRulesStagedReadsIndexNotDisk`, `TestVerifyBacklogRulesStagedIgnoresInheritedGitDir` y `TestVerifyBacklogRulesStagedWithoutGit` en `internal/verify/backlog_test.go`; `TestVerifyStagedReadsIndex` en `cmd/aquas/main_test.go`.
- Códigos de salida y formato de mensajes: `TestVerifyReportsViolations`, `TestVerifyBadArgsIsCode2`, `TestVerifyWithoutBacklogIsCode3` y `TestVerifyGreenAndFromSubfolder` en `cmd/aquas/main_test.go`.
- Decodificador estricto: `TestDecodeRejectsUnknownFields` en `internal/verify/verify_test.go`, con una subprueba por cada rechazo; `TestBaseFixtureIsGreen` para el caso verde.
- Los archivos reales del repo decodifican y no tienen violaciones: `TestRepoFilesDecode`.
- JSON Schema publicados: `TestSchemasMatchTypes`, `TestSchemaStaleIsDetectedByName`, `TestGenerateChangesWithTypes` y `TestGeneratedSchemaShape` en `internal/schema/schema_test.go`.
- Reglas del backlog: `TestVerifyBacklogRules` en `internal/verify/backlog_test.go`.
- Reglas de capacidades y páginas: `TestVerifyCapabilityPages` en `internal/verify/capabilities_test.go`.
- `install` exige verde, revierte y no deja nada: `TestInstallRunsVerify` en `internal/install/verify_test.go`.
- El ciclo completo de F2, con la instalación limpia, una fila por regla y la plantilla rota: `TestIntegrationVerify` en `cmd/aquas/integration_test.go`.

Sin prueba, según esta página: el caso `Foo.json`, `null` en `decisions` y las claves repetidas dentro de `decisions`. Tampoco tienen prueba el mensaje de `git` ausente, el valor 0 de `priority`, el enlace simbólico agregado al índice y que `update` no llame al verificador.
