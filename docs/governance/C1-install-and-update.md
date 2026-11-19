# C1. Instalar y actualizar

Esta página describe lo que hace el código hoy, no lo que promete el plan. Cada afirmación cita el archivo y la función donde está. Cita la prueba que la cubre cuando existe; si no existe, lo dice con «Sin prueba».

Los niveles de evidencia son los de `canon/communication.md`. Aquí todo está `verificado` contra el código el 2026-11-18, salvo lo marcado como `hipótesis` o `desconocido`. Las rutas de código (`cmd/` e `internal/`) son relativas a `tools/`. Las demás rutas (`canon/`, `templates/`, `skill/`, `work/`, `.github/`) son relativas a la raíz del paquete.

## Para qué sirve C1

C1 es la capacidad de poner Aquas Light en un proyecto y mantenerla al día. Aquas Light es el paquete de reglas, herramientas y flujo de trabajo que se copia a la carpeta del proyecto.

F1 es la feature (el entregable del plan) que construyó C1. La instancia es la carpeta del proyecto donde se instaló Aquas Light. El paquete es el origen desde donde se instala. `golangci-lint` es el programa que revisa el estilo del código Go. El preflight es la revisión de requisitos previa a instalar. El rollback es deshacer lo creado cuando un paso falla.

La capacidad se ejerce con el programa `aquas`. Sus subcomandos son `install`, `update`, `host`, `setup` y `version` (`cmd/aquas/main.go`, función `run`).

La skill `skill/SKILL.md` permite que Claude Code corra esos comandos por el humano. Se describe más abajo.

## Cómo se instala

El comando exacto es `go run ./cmd/aquas install <ruta>`. Se corre con el directorio actual dentro del paquete, normalmente en `tools/` (`skill/SKILL.md`, paso 4).

El directorio actual puede ser cualquiera dentro del paquete. `aquas` sube por las carpetas hasta encontrar una que tenga `canon/` y `tools/go.mod` (`internal/install/copy.go`, función `FindPackageRoot`). Esa carpeta es la raíz del paquete. Prueba: `TestFindPackageRoot`.

El subcomando exige exactamente un argumento, la ruta destino. Sin él sale con código 2 (`cmd/aquas/main.go`, función `runInstall`).

Los pasos de `install` corren en este orden (`internal/install/install.go`, función `Run`):

1. Preflight, que valida los requisitos.
2. `git init`.
3. Copia de archivos y escritura del manifiesto.
4. Compilación de `bin/aquas`.
5. Descarga de `golangci-lint`.
6. Escritura de `work/host.json`.
7. Commit de instalación.

Prueba del conjunto: `TestInstallSuccess` y `TestIntegrationInstall`, parte (a).

### Qué valida el preflight

El preflight (la revisión previa) está en `internal/install/preflight.go`, función `Preflight`. Revisa tres cosas, en este orden.

Primero, que `git` esté en el `PATH`. Segundo, que Go sea 1.24 o superior; lo lee con `go env GOVERSION` (`DefaultDeps`) y lo compara en `checkGo`. Prueba: `TestPreflightMissingGit` y `TestPreflightGoVersion`.

Si faltan `git` y Go, el error lista ambos problemas juntos. Si falta Go, el mensaje agrega el texto en inglés que devuelve el sistema, porque `checkGo` incluye el error original de `exec`.

Tercero, la carpeta destino. Si no existe, la crea junto con las carpetas padre que falten. Si existe, debe estar completamente vacía: un `.gitkeep` o un `.git` vacío ya cuentan como contenido y la rechazan. Si no es una carpeta, la rechaza. Prueba: `TestPreflightRejectsNonEmptyFolder` y `TestPreflightCreatesMissingAndAcceptsEmpty`.

Si el preflight falla, `install` sale con código 1, imprime un mensaje en español y no deja nada creado. Si el preflight creó la carpeta y falla después, la borra (`Preflight`, bloque `defer`). El código de salida 1 lo mide `TestIntegrationInstall`, parte (b). `TestPreflightMissingGit` mide solo que la ruta no se crea.

### Qué copia

La copia está en `internal/install/copy.go`, función `Copy`. Prueba: `TestCopyManifestDeterministicAndConsistent`.

- Copia `canon/` y `tools/` completas, byte por byte, sin la carpeta `tools/bin`.
- Copia `schemas/` solo si existe en el paquete. Hoy el paquete no tiene `schemas/`, así que no se copia.
- Copia los archivos de `templates/` a la raíz de la instancia: `backlog.json`, `README.md`, `.gitignore` y `.gitattributes`. Las dos últimas se guardan en el paquete sin el punto inicial y `dotNames` les pone el nombre final. Prueba: `TestCopyStarterFiles`.
- Crea las carpetas `work/`, `archive/`, `product/` y `docs/`, cada una con un `.gitkeep` vacío (función `writeStarter`).
- Escribe `canon/manifest.json`.

El archivo de plantilla `.gitattributes` contiene `* text=auto eol=lf`, es decir, fuerza finales de línea LF en la instancia (`templates/gitattributes`).

El manifiesto lista una huella SHA-256 por cada archivo copiado de `canon/`, `schemas/` y `tools/`, sin contar el propio manifiesto. La huella se calcula después de convertir los finales CRLF en LF (`internal/hashing/hash.go`, funciones `Normalize` y `Sum`). Pruebas: `TestManifestHashIgnoresCRLF` y `TestHashOnlyNormalizesLineEndings`.

El manifiesto tiene `schema_version` 1 y `algorithm` `sha256-lf`. Las rutas usan `/` y salen ordenadas, porque `json.Marshal` ordena las claves de un mapa (`writeManifest`). Prueba: `TestCopyManifestDeterministicAndConsistent`.

El manifiesto tiene además el campo `version`, con la versión del paquete sin la «v», por ejemplo `0.1.0` (`Manifest` y `writeManifest` en `copy.go`). `install` lo escribe al copiar. Un manifiesto viejo sin el campo se lee sin error. Pruebas: `TestManifestRecordsVersion` y `TestManifestWithoutVersionIsReadAndFilled`, en `internal/update/update_test.go`.

### Qué commit deja

`install` deja un solo commit con el asunto `Install Aquas Light v0.1.0` (`internal/install/git.go`, función `commitInstall`). La versión sale de la constante `version` en `cmd/aquas/main.go`. Prueba: `TestIntegrationInstall`, parte (a), que comprueba un commit, su asunto y el árbol limpio.

El commit se hace con `-c core.hooksPath=<carpeta vacía>`. Así no corren los ganchos globales del usuario. También usa `-c commit.gpgsign=false`, para que no pida firma. Prueba: `TestInstallIgnoresGlobalHooks`.

`git init` usa `--template=` para que ninguna plantilla de git agregue ganchos al repositorio nuevo (`gitInit`). Sin prueba.

El commit falla solo si git no logra obtener una identidad, ni configurada ni autodetectada (`commitInstall`). Entonces el mensaje explica cómo configurarla y `install` revierte todo (`errNoIdentity` en `install.go`). La prueba acepta los dos desenlaces: commit único o error que nombra `user.name`. Prueba: `TestInstallWithoutGitIdentity`.

### `work/host.json` y por qué está ignorado

`work/host.json` guarda el sistema operativo, la arquitectura, el shell, el estilo de rutas y el fin de línea. También guarda las versiones de `go` y `git` y las reglas de `canon/hosts.json` para ese sistema (`internal/host/detect.go`, función `Write`). Prueba: `TestDetectHostWritesHints`.

Si el sistema no está en `canon/hosts.json`, `Write` falla y deja intacto el `host.json` anterior. Hoy el archivo cubre `linux`, `darwin` y `windows`. Prueba: `TestUnknownOSKeepsPreviousHost`.

El archivo se escribe en un temporal y se renombra, para que nunca quede a medias. Prueba: `TestConcurrentWrites`.

`templates/gitignore` contiene dos líneas: `bin/` y `work/host.json`. El archivo describe una máquina concreta, así que no se guarda en git. Ignorarlo también evita que `setup` deje cambios sin confirmar en el repositorio. Por eso el commit de instalación no lo incluye, aunque el archivo queda en disco. Prueba: `TestIntegrationInstall`, que comprueba el archivo en disco y el árbol limpio (`git status --porcelain` vacío). 

### Descarga verificada de `golangci-lint`

La descarga está en `internal/install/lintdownload.go`, función `downloadLint`. La versión fijada es 2.5.0, igual que `canon/toolchain.json`.

La función arma la URL de la release oficial en GitHub para el sistema y la arquitectura de la máquina. Las huellas SHA-256 esperadas están escritas en el código (`pinnedLintSHA256`). El comentario del código dice que vienen del archivo de huellas oficial del release v2.5.0.

Si la huella del archivo descargado no coincide, la función falla y el mensaje nombra la huella esperada y la recibida. El binario no queda en disco. Prueba: `TestInstallLintChecksumMismatch`.

Si el servidor responde algo distinto de 200, la función falla con el código recibido. Prueba: `TestInstallLintServerError`.

Las plataformas con huella fijada son `linux`, `darwin` y `windows`, cada una en `amd64` y `arm64`. Otra combinación falla con «no hay golangci-lint fijado». Sin prueba.

Solo después de verificar la huella, la función extrae el binario del `.tar.gz` o `.zip`. Lo escribe en `bin/`, primero como `.tmp` y luego renombrado. `TestInstallSuccess` cubre la extracción con un archivo falso. El paso por `.tmp` no tiene prueba.

Limitación de evidencia, `desconocido`: ninguna prueba descarga el archivo real desde GitHub. Las pruebas usan un servidor local falso con huellas inyectadas. Que las huellas copiadas coincidan con el release real no está probado por el código.

### Rollback

Si cualquier paso después del preflight falla, `Run` llama a `rollback` (`internal/install/rollback.go`). Si `install` creó la carpeta, la borra junto con las carpetas padre que creó. Si la carpeta ya existía vacía, borra su contenido y la deja vacía.

El borrado quita antes el bit de solo lectura, porque en Windows los archivos de `.git/objects` no se pueden borrar de otro modo (`removeAll`). Pruebas: `TestInstallRollsBackOnFailure` y `TestInstallRollbackRemovesReadOnlyGit`.

Al fallar, el comando imprime en `stderr` «no se puede instalar (se revirtió todo lo creado)» y sale con código 1 (`runInstall`).

## Cómo se actualiza

El comando es `go run ./cmd/aquas update <instancia>`. Se corre desde el paquete nuevo, con el directorio actual dentro de él, igual que `install`. La instancia es la carpeta ya instalada (`cmd/aquas/main.go`, función `runUpdate`).

`update` no hace commit ni recompila `bin/aquas`. Los cambios quedan sin confirmar en el repositorio de la instancia (`internal/update/update.go`, función `Update`: solo escribe y borra archivos).

### Qué reemplaza

`update` deja iguales al paquete estas carpetas: `canon/`, `schemas/`, `tools/`, `.githooks/`, `templates/` y `docs/governance/` (variable `mirrored`). Los archivos nuevos se crean, los distintos se reemplazan y los que el paquete ya no trae se borran. `tools/bin/` se salta.

Para saber si la instancia es válida, `update` lee `canon/manifest.json`. Si no existe o no se puede leer, falla con «no es una instancia de Aquas Light». Sin prueba.

No toca `backlog.json`, `product/`, `docs/product/` ni el resto de `work/`. Prueba: `TestUpdateNeverTouchesProductAndReplacesGovernance`. Prueba del borrado y de `tools/bin/`: `TestUpdateDeletesDroppedAndSeals`.

`update` también toca `work/seals.json`. Reemplaza solo las entradas cuya clave empieza con `docs/governance/` (`updateSeals`). Nunca crea ese archivo. Prueba: `TestUpdateDeletesDroppedAndSeals` y `TestUpdateDoesNotCreateSeals`. El formato real de `work/seals.json` es una `hipótesis` hasta la feature F6, la que lo define. El código acepta un objeto con las claves `seals` o `files`, o el objeto raíz.

Al terminar, reescribe `canon/manifest.json` con la versión del paquete nuevo y las huellas de `canon/` y `tools/` (`update.go`, `Update`). Si queda algún conflicto sin resolver, no escribe nada y la versión anterior se conserva. Prueba: `TestManifestVersionUnchangedOnConflict`. Un manifiesto sin versión recibe la nueva en el primer `update` que termina sin conflictos. No guarda las de `schemas/`, que `install` sí guarda (`copy.go`, `Copy`). Tampoco guarda las de `templates/`, `.githooks/` ni `docs/governance/`. Sin prueba.

### Conflictos

Solo hay conflictos en `canon/`. Un archivo de `canon/` es un conflicto en dos casos juntos (`Update`, paso 1). Su huella actual no coincide con la del manifiesto. Además, es distinto de la versión del paquete. La comparación ignora CRLF contra LF. Prueba: `TestUpdateCRLFOnlyIsNotConflict`.

Un archivo de `canon/` que no está en el manifiesto también es conflicto, si difiere del paquete o el paquete ya no lo trae. Sin prueba.

Los archivos de `tools/`, `templates/`, `.githooks/` y `docs/governance/` no tienen detección de conflictos. Si el humano los editó, `update` los sobrescribe sin avisar. El paso 1 de `Update` solo recorre rutas que empiezan con `canon/`. Sin prueba.

Si hay conflictos sin resolver, `update` no escribe nada y falla con un error que lista los archivos (`ConflictError`). Prueba: `TestUpdateStopsOnLocalEdit` y `TestUpdateConflictChangesNothing`. El archivo sin conflicto tampoco se cambia en esa corrida.

### Opciones `--take`

- `--take ours` o `--take theirs` aplica a todos los conflictos.
- `--take <ruta>=ours` o `--take <ruta>=theirs` aplica a un solo archivo. La ruta se escribe con `/`, y un `\` se cambia a `/`. También se acepta la forma `--take=<valor>`.
- Si un archivo tiene opción propia, esa se usa en lugar de la general. Se pueden repetir.

`ours` conserva la versión de la instancia. `theirs` toma la del paquete. Fuente: `cmd/aquas/main.go`, función `parseUpdateArgs`, y `internal/update/update.go`, función `resolve`.

`ours` tiene un efecto que conviene saber. El manifiesto se reescribe con la huella del paquete, no con la del archivo conservado. Por eso el mismo conflicto reaparece en la siguiente actualización. Prueba: `TestUpdateOursKeepsAndReappears`.

`--take` aplica solo a archivos en conflicto. Con una ruta que no está en conflicto, la opción no hace nada (`Update`, paso 1: `resolve` se llama solo para conflictos). Sin prueba.

### Códigos de salida

- 0: la actualización terminó. Imprime cuántos archivos cambiaron y cuántos estaban en conflicto.
- 1: hay conflictos sin resolver, o falló algo más (la instancia no es válida, no se encontró el paquete, un error al escribir). Imprime «no se actualizó nada» seguido del motivo.
- 2: los argumentos son inválidos. Incluye un valor de `--take` distinto de `ours` o `theirs`, una opción desconocida, un argumento sobrante o la falta de la ruta de la instancia.

Fuente: `runUpdate` y `parseUpdateArgs`. Pruebas: `TestUpdateInvalidTakeExits2` (código 2) y `TestIntegrationInstall`, partes (c) y (c2) (código 1 y 0).

El mensaje «no se actualizó nada» es exacto solo para los conflictos y los errores de lectura. Si ocurre un error de escritura a mitad de la copia, algunos archivos ya se escribieron. Esta lectura es `inferido` del orden de pasos en `Update`; ninguna prueba la cubre.

## `aquas setup` en un clon

`setup` prepara un clon de una instancia en otra máquina. Se corre con `go run ./cmd/aquas setup` desde `tools/` dentro del clon (`cmd/aquas/main.go`, función `runSetup`; `internal/install/setup.go`, función `Setup`).

Hace cuatro cosas, en este orden:

1. Exige que la carpeta tenga `.git`. Si no, falla con un mensaje en español y no crea `.git`.
2. Compila `bin/aquas`.
3. Baja `golangci-lint` con la misma verificación de huella que `install`.
4. Escribe `work/host.json` y ejecuta `git config --local core.hooksPath .githooks`.

`setup` no crea commits y se puede repetir: dos corridas seguidas dejan el mismo `HEAD` y el árbol limpio. Pruebas: `TestSetupPreparesClone`, `TestSetupRequiresGit`, `TestSetupChecksumMismatch`, `TestSetupLintServerError`, `TestSetupSetsLocalHooksPath` y `TestIntegrationInstall`, parte (d).

Hoy la instancia no trae una carpeta `.githooks/`: el paquete no la tiene. `setup` igualmente apunta `core.hooksPath` a esa ruta. Los ganchos los agregan slices posteriores (S40 y S42, que `work/specs/S5.json` menciona como pendientes).

## La skill y la etiqueta `v0.1.0`

`skill/SKILL.md` es la skill `aquas-light`. Le indica a Claude Code estos pasos para instalar o actualizar:

1. Pedir la carpeta nueva, o la ruta de la instancia.
2. Revisar `go env GOVERSION` y `git --version`. En Windows exige Git for Windows y que los comandos corran desde Git Bash.
3. Clonar el paquete en un directorio temporal con `git clone --depth 1 --branch v0.1.0 https://github.com/jliberal-aquas/aquas-light`.
4. Correr `install` o `update` desde `<temporal>/tools`.
5. Si `update` sale con código 1, preguntar al humano por cada archivo en conflicto y repetir con `--take`.
6. Borrar el directorio temporal.
7. Indicar al humano cómo abrir Claude Code en la carpeta nueva.

La skill nunca crea ni sube repositorios a GitHub.

La etiqueta `v0.1.0` todavía no existe: `git tag` local no lista etiquetas y `git ls-remote --tags origin` no devuelve ninguna. Sin la etiqueta, el paso 3 de la skill falla. La skill manda decírselo al humano y detenerse. La regla 11 de `canon/constitution.md` prohíbe publicar etiquetas sin orden del humano, así que la crea el humano.

Por eso la prueba funcional de la skill, hecha por el humano con Claude Code, está pendiente (`desconocido`). Ningún test automático corre la skill.

## Plataformas

El código contempla Linux, macOS y Windows:

- `bin/aquas.exe` y `golangci-lint.exe` en Windows (`buildAquas`, `downloadLint`).
- `.zip` en Windows y `.tar.gz` en los otros dos (`lintSuffix`).
- Shell `git-bash` en Windows, `zsh` en macOS y `bash` en Linux (`internal/host/detect.go`, `shellTool`).
- Reintentos al renombrar `work/host.json` en Windows (`writeAtomic`).

La configuración de integración continua corre `gofmt`, `go vet`, `golangci-lint` y `go test -race` en `ubuntu-latest`, `macos-latest` y `windows-latest` (`.github/workflows/ci.yml`). La configuración corre también `golangci-lint`. La corrida 44447914168 de GitHub Actions terminó en verde en los tres sistemas.

## Limitaciones conocidas

1. `install` no deja `templates/` ni `docs/governance/` en la instancia. El primer `update` las crea, porque están en `mirrored`. `Copy` solo copia `canon/`, `schemas/` y `tools/`. `TestUpdateNeverTouchesProductAndReplacesGovernance` lo da por hecho. Está sin decidir si la instancia debe tenerlas desde `install`.
2. Si falta Go, el mensaje del preflight incluye el texto en inglés del error original.
3. `canon/delegation.md` cita `schemas/report.schema.json`, pero ese archivo no existe en el paquete. Ningún informe se valida contra un esquema.
4. Si la descarga de `golangci-lint` falla en `setup`, `bin/aquas` queda en disco. `bin/` está ignorado, así que git no lo lista como cambio (`Setup` solo limpia `bin/` cuando falla la compilación).
5. El formato de `work/seals.json` es una `hipótesis` hasta F6.
6. La prueba funcional de la skill está pendiente hasta que exista la etiqueta `v0.1.0`.
7. `update` sobrescribe sin avisar las ediciones locales de `tools/`, `templates/`, `.githooks/` y `docs/governance/`.
8. `update` no recompila `bin/aquas`: tras actualizar, el binario sigue siendo el anterior hasta correr `setup` o compilar a mano.
9. Las huellas de `golangci-lint` nunca se contrastaron en las pruebas con el servidor real de GitHub.

## Qué prueba cubre cada afirmación

Todas las pruebas están en `tools/` y corren con `go test ./...`.

- Preflight: `TestPreflightRejectsNonEmptyFolder`, `TestPreflightCreatesMissingAndAcceptsEmpty`, `TestPreflightMissingGit`, `TestPreflightGoVersion` en `internal/install/preflight_test.go`.
- Copia y manifiesto: `TestCopyBinaryIdentical`, `TestCopyManifestDeterministicAndConsistent`, `TestCopyStarterFiles`, `TestFindPackageRoot` en `copy_test.go`; `TestManifestHashIgnoresCRLF` y `TestHashOnlyNormalizesLineEndings` en `internal/hashing/hash_test.go`; `TestManifestRecordsVersion`, `TestManifestWithoutVersionIsReadAndFilled` y `TestManifestVersionUnchangedOnConflict` en `internal/update/update_test.go`.
- Instalación completa, commit y rollback: `TestInstallSuccess`, `TestInstallRollsBackOnFailure`, `TestInstallLintServerError`, `TestInstallLintChecksumMismatch`, `TestInstallRollbackRemovesReadOnlyGit`, `TestInstallWithoutGitIdentity`, `TestInstallIgnoresGlobalHooks` en `install_test.go`.
- Host: `TestDetectHostWritesHints`, `TestShellAndPathStyle`, `TestUnknownOSKeepsPreviousHost`, `TestNonJSONPreviousIsReplaced`, `TestConcurrentWrites`, `TestBrokenHostsJSON` en `internal/host/detect_test.go`.
- Actualización: las nueve pruebas `TestUpdate*` de `internal/update/update_test.go`.
- Setup: las cinco pruebas `TestSetup*` de `internal/install/setup_test.go`.
- Subcomandos y códigos de salida: `TestVersion`, `TestNoArgs`, `TestUnknownSubcommand`, `TestHostSubcommand`, `TestUpdateInvalidTakeExits2`, `TestSetupSubcommandOutsideRepo` en `cmd/aquas/main_test.go`.
- El ciclo completo de la feature F1 (instalar, rechazar carpeta con archivo, conflicto, `--take`, clon y `setup`): `TestIntegrationInstall` en `cmd/aquas/integration_test.go`.

La prueba de integración usa un servidor local para `golangci-lint`. No toca la red.
