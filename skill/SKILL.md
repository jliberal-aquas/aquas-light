---
name: aquas-light
description: Instala o actualiza Aquas Light en un proyecto. Úsala cuando el humano pida instalar Aquas Light en una carpeta nueva, o actualizar una instancia que ya tiene instalada.
---

# Aquas Light

Esta skill instala Aquas Light (un paquete de reglas, herramientas y flujo de trabajo para Claude Code) en una carpeta nueva, o actualiza una instalación existente. El humano no escribe comandos: los corres tú.

La skill nunca crea ni sube repositorios a GitHub. Solo crea un repositorio git local dentro de la carpeta nueva.

Los comandos van en inglés y exactos. Habla con el humano en español venezolano neutro, con tuteo, con oraciones de máximo 25 palabras.

## Paso 1. Pedir la carpeta

- Si es una instalación, pregunta el nombre de la carpeta nueva (por ejemplo `mi-api`). La carpeta no debe existir, o debe estar vacía.
- Resuélvela a ruta absoluta dentro del directorio actual. Llámala `<ruta>`.
- Si es una actualización, pregunta la ruta de la instancia ya instalada. Llámala `<instancia>`.

## Paso 2. Revisar requisitos

Corre estos comandos:

```
go env GOVERSION
git --version
```

- Go debe ser 1.24 o superior.
- En Windows, además, debe estar instalado Git for Windows (trae Git Bash). Los comandos se corren desde Git Bash.
- Si falta algo, dile qué instalar y de dónde, y detente. No sigas hasta que el humano lo confirme.

## Paso 3. Bajar el paquete fijado

Baja el paquete fuera de la carpeta destino, en un directorio temporal:

```
tmp="$(mktemp -d)"
git clone --depth 1 --branch v0.1.0 https://github.com/jliberal-aquas/aquas-light "$tmp"
```

Si git dice que la etiqueta `v0.1.0` no existe, díselo al humano con esas palabras y detente. No uses otra rama ni otra versión.

## Paso 4. Correr el comando

Con el directorio actual en `<temporal>/tools`:

```
cd "$tmp/tools" && go run ./cmd/aquas install <ruta>
cd "$tmp/tools" && go run ./cmd/aquas update <instancia>
```

Usa el primero para instalar y el segundo para actualizar. `<ruta>` e `<instancia>` son rutas absolutas.

`install` crea el repositorio git, copia los archivos, compila, baja `golangci-lint` y hace el commit `Install Aquas Light v0.1.0`. Si algo falla, revierte y deja la carpeta como estaba. Muestra al humano el error tal cual, explicado en una o dos oraciones.

## Paso 5. Resolver conflictos de update

Si `update` sale con código 1, lista los archivos del canon (`canon/`) que el humano editó en su instancia. Por cada archivo, haz una pregunta con este formato:

```
Qué necesito de ti

Qué pasa: el archivo `<archivo>` lo editaste tú y la versión del paquete es distinta de la tuya. Ejemplo concreto: <una línea de la diferencia>.
Por qué importa: <qué se rompe o se pierde según lo que elijas>.
Opciones:
- ours (conservar tu edición): gana <lo que conservas>. Pierde <lo nuevo del paquete en ese archivo>.
- theirs (tomar la del paquete): gana <lo nuevo del paquete>. Pierde <tu edición>.
Recomiendo <ours o theirs> porque <razón>.
Pregunta: ¿conservo tu edición de `<archivo>` o tomo la del paquete?
```

No pidas aceptar nada por referencia: repite el contenido de cada opción. Mira la diferencia real del archivo antes de redactar el ejemplo.

Con las respuestas, vuelve a correr el comando con una opción `--take` por archivo:

```
cd "$tmp/tools" && go run ./cmd/aquas update <instancia> --take <ruta-del-archivo>=ours --take <otra-ruta>=theirs
```

Si el humano quiere lo mismo para todos, usa `--take ours` o `--take theirs` sin ruta. Si el comando sale con código 2, el valor de `--take` es inválido: corrígelo y repite.

## Paso 6. Borrar el temporal

Al terminar, con éxito o con error:

```
rm -rf "$tmp"
```

## Paso 7. Cerrar con el humano

Si todo salió bien, dile en pasos numerados cómo seguir:

1. Abre una terminal.
2. Escribe `cd <ruta>` y pulsa Enter.
3. Escribe `claude` y pulsa Enter para abrir Claude Code en esa carpeta.
4. Dile a Claude qué quieres construir.

Usa la ruta real que resolviste. No ofrezcas crear ni subir un repositorio a GitHub: esta skill no lo hace.
