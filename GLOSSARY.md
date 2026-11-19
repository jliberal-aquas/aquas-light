# Aquas Light

Paquete portable que instala en un repo vacío el gobierno de Aquas: reglas, ganchos, herramientas y estructura. Existe para que el humano defina y apruebe el trabajo sin perder el hilo, con el menor gasto de contexto posible.

## Trabajo

**Iniciativa**:
Objetivo de negocio que agrupa features. No se construye directamente; se construye a través de sus features.
_Avoid_: épica, proyecto

**Feature**:
Parte de una iniciativa con una sola línea de aceptación que el humano puede observar. Cierra con pruebas integrales, code review y auditoría.
_Avoid_: historia, funcionalidad

**Slice**:
Unidad mínima completable de una feature: se especifica, se prueba en rojo, se implementa, se prueba en verde y se cierra sola. Un slice cerrado nunca se modifica.
_Avoid_: tarea, ticket, slide

**Bug**:
Falla observada en un slice cerrado. Es una entrada propia que apunta a ese slice y nace con una prueba roja que reproduce la falla.
_Avoid_: incidencia, defecto (defecto es la falla del gobierno que anota `work/defects.md`)

**Necesidad**:
Pedido del humano que todavía no está refinado. Vive en el backlog en estado `to_refine` con la ruta a sus palabras exactas.
_Avoid_: inbox, pendiente por definir

**Cambio de alcance**:
Necesidad que modifica algo ya acordado. Nunca edita un slice cerrado: crea un slice o una feature nueva que declara a cuál reemplaza.

**Capacidad**:
Lo que el producto sabe hacer, contado para el humano. Es la unidad de la wiki; una capacidad puede reunir varias features.
_Avoid_: módulo, feature (en la wiki)

**Barrido**:
Revisión del backlog por prioridad al cerrar un slice: primero los bugs de la feature en curso. Las necesidades se refinan al cerrar la feature.

## Gobierno

**Gobierno**:
Todo lo que instala el paquete: canon, herramientas en Go, ganchos, esquemas, `work/` y `archive/`. Tiene sus propios linters desde el primer día.
_Avoid_: framework, harness

**Producto**:
Lo que el repo construye para su usuario, dentro de `product/`. Define sus propios linters y pruebas según su lenguaje.
_Avoid_: app, proyecto

**Lado**:
Pertenencia de un archivo al gobierno o al producto. Decide qué linters, pruebas y extractor de funciones se le aplican.

**Componente**:
Parte del producto con su propio lenguaje y comandos `test`, `lint`, `format` y `symbols`, como una API en FastAPI.
_Avoid_: servicio, módulo

**Canon**:
Fuente única de las reglas, roles y configuraciones. Todo archivo que lee el arnés se compila desde aquí y no se edita a mano.
_Avoid_: config, settings

**Arnés**:
Programa que ejecuta al modelo y sus agentes, como Claude Code, Oh My Pi o Codex. El canon no depende de ninguno.

**Configuración del repo**:
Archivo `canon/config.json`, que se sube al repo y vale en toda máquina: rol del repo, modelo por defecto e idioma. Los componentes del producto viven en `product/toolchain.json`.
_Avoid_: settings, config global

**Configuración del host**:
Archivo `work/host.json`, propio de cada máquina y fuera del repo. Dice sistema, shell, estilo de rutas y versiones, y cada sesión lo regenera y se lo inyecta a todos los agentes.
_Avoid_: entorno, env

**Carpeta vacía**:
Único destino válido de la instalación: una carpeta que no existe o que no tiene ningún archivo, ni siquiera `.git`. La skill corre fuera de ella, la crea si falta y corre `git init`; si trae algo, no crea nada.
_Avoid_: repo vacío

**Instancia**:
Repo donde se instaló el paquete. Se gobierna sola, sin depender del repo central.

**Backlog**:
Lista liviana de lo pendiente en `backlog.json`: ID, padre, tipo, estado, prioridad y ruta. El detalle vive en su especificación.

**Especificación**:
Archivo con el detalle de una iniciativa, feature o slice: qué hace, qué archivos toca y sus pruebas. Solo entra al contexto cuando se trabaja.
_Avoid_: spec suelta, ticket

**Memoria caliente**:
Archivo que dice en qué punto exacto está el slice en curso, para retomar tras un corte. Al cerrar el slice se archiva.
_Avoid_: hot memory, notas

**Archivo histórico**:
Carpeta `archive/` con lo terminado: features cerradas, memorias y registros. No entra al contexto salvo para auditar.

**Caso de desvío**:
Registro en `work/gate-bypass.jsonl` de un control que dejó pasar un segundo intento o de un commit que saltó los ganchos. Mientras esté abierto bloquea empezar y cerrar slices; se cierra con una razón escrita que lee el auditor.
_Avoid_: bypass, excepción

## Documentación

**Referencia narrada**:
Página que explica un archivo de código función por función: para qué sirve, qué recibe, qué devuelve, qué errores da y un ejemplo. La prosa la escribe el modelo; la lista de funciones la comprueba la máquina.
_Avoid_: docstring, API doc

**Sello**:
Registro que ata un documento a lo que describe mediante la huella de ambos. Si lo descrito cambia, el documento queda vencido.
_Avoid_: firma, candado

**Documento vencido**:
Documento cuyo sello ya no coincide con lo que cubre, directamente o por la cadena bug, slice, feature, iniciativa y capacidad. Bloquea el commit hasta resellarlo.
_Avoid_: desactualizado (es vago)

**Resello**:
Nuevo sello de un documento vencido. Vale solo si el documento cambió o si trae la razón escrita de por qué sigue siendo cierto.
