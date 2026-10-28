# MOD-ARCH

[English](README.md)

MOD-ARCH es un simulador educativo para enseñar sistemas operativos sobre una máquina x86 de 32 bits simplificada. Ejecuta programas codificados a mano paso a paso y muestra en vivo lo que un sistema operativo hace sobre la CPU: cambios de modo kernel/usuario, interrupciones de hardware (timer, teclado), llamadas al sistema (syscalls), preservación del bloque de control de proceso (PCB) en los cambios de contexto, planificación, sincronización y violaciones de memoria. El backend está escrito en Go y el frontend en Vue 3. Incluye 30 demos curriculares, un modo de compartir en solo lectura para clases, un panel de administración, interfaz en español, inglés y portugués, y un tutor opcional basado en generación aumentada por recuperación (ARCHIUS).

**Demo en línea:** https://mod-arch-simulator.com

## Paper y reconocimientos

MOD-ARCH se presentó en la XXI Conferencia Latinoamericana de Tecnologías del Aprendizaje (LACLO 2026), en Río de Janeiro:

- J. Juárez, A. Morales Bezeira, A. Silva Sprock. *MOD-ARCH 1.0: An Open-Concept Educational Tool for Teaching Operating Systems*. LACLO 2026. En prensa en Springer Lecture Notes in Educational Technology.
- Tercer lugar del concurso de objetos de aprendizaje de LACLO 2026 con *MOD-ARCH: A Laboratory for Operating Systems Practical Sessions* ([lista oficial de trabajos premiados](https://lacloconference.com/awarded-papers/)).

## Autores

- Jeanmarcos Juárez
- Ana Morales Bezeira
- Antonio Silva Sprock

## Características

- **CPU**: un subconjunto de x86 de 32 bits con dos niveles de privilegio (PL0 kernel, PL3 usuario), una IDT, regiones de memoria con permisos verificados y un evento tipado por cada efecto observable de una instrucción. De uno a cuatro núcleos simulados.
- **Kernel**: PCB, planificador round-robin guiado por un quantum de timer configurable, `fork`, `vfork`, `clone`, `waitpid`, pipes, mutex y semáforos, `read` bloqueante y una tarea ociosa opcional.
- **Demos**: 30 programas codificados a mano, cada uno con su fuente en pseudo-C, un mapa de fuente a instrucciones y un panel de variables.
- **Visualización**: vistas en vivo de procesos, memoria, interrupciones y concurrencia, bitácora de eventos, glosario integrado, cuatro temas de color, tres modos de presentación (compacto, pedagógico, proyector) y un menú de accesibilidad.
- **Idiomas**: español, inglés y portugués.
- **Sesiones**: cada visitante obtiene un simulador aislado. La capacidad y el tiempo de inactividad son configurables.
- **Compartir**: el dueño de una sesión puede generar un enlace `/v/<token>` de solo lectura. Los espectadores ven los mismos eventos en tiempo real, pero no pueden ejecutar comandos. Cuando se alcanza el tope de sesiones públicas, `/full` lista las sesiones compartidas a las que es posible unirse como espectador. Desde el panel de administración puede exigirse una contraseña para los enlaces de espectador.
- **Inicio de sesión opcional para estudiantes**: nombre, correo y carnet o matrícula, con enlace mágico por correo cuando hay SMTP configurado, lista de correos permitidos y verificación opcional con Cloudflare Turnstile.
- **Panel de administración** (desactivado por defecto): sesiones activas e históricas, terminar, etiquetar y publicar sesiones, usuarios y roles (estudiante, moderador, administrador), enlaces de invitación privilegiados, modo de acceso (abierto, solo por invitación, cerrado), activación o desactivación de demos, ventanas programadas de interfaz, valores por defecto de la interfaz, configuración SMTP, bitácora de auditoría, reportes de usuarios, métricas de uso y del tutor, y configuración del tutor.
- **Tutor (ARCHIUS)**: opcional; véase la sección Tutor. El simulador funciona completo sin él.

## Stack

- **Backend**: Go. Un binario único que embebe el frontend compilado (`internal/staticui`). SQLite mediante el controlador `modernc.org/sqlite`, escrito en Go puro, por lo que la compilación funciona con `CGO_ENABLED=0`.
- **Frontend**: Vue 3, Vite, TypeScript, Pinia, vue-router y vue-i18n.
- **Transporte**: WebSocket. El backend emite eventos atómicos de la CPU y el frontend reconstruye su estado a partir del flujo.
- **Persistencia**: un único archivo SQLite con usuarios, configuración, historial de sesiones, eventos de uso y el corpus del tutor.

## Estructura del repositorio

| Ruta | Contenido |
|---|---|
| `cmd/modos-cpu/` | Punto de entrada del servidor y subcomandos `cli` y `admin-hash` |
| `cmd/modos-rag-index/` | Construye el corpus del tutor (fragmentos y embeddings) dentro del archivo SQLite |
| `cmd/ts-gen/` | Genera `web/src/types/generated.ts` a partir de structs de Go |
| `pkg/cpu/` | CPU: registros, memoria con permisos, ISA, interrupciones, eventos |
| `pkg/kernel/` | Capa de sistema operativo: PCB, planificador, syscalls, IPC, sincronización |
| `internal/sim/` | Simulador que envuelve el kernel y emite eventos al WebSocket |
| `internal/sessions/` | Gestor de sesiones por visitante (topes, limpieza por inactividad, compartir) |
| `internal/server/` | HTTP, WebSocket, autenticación y API de administración |
| `internal/programs/` | Catálogo de demos y un ensamblador pequeño (`asm/`) |
| `internal/tutor/`, `internal/rag/`, `internal/llm/` | Orquestación del tutor, recuperación y clientes LLM |
| `internal/store/` | Persistencia SQLite (esquema en `schema.sql`) |
| `internal/staticui/` | Embebe el frontend compilado |
| `web/` | Frontend Vue 3 |
| `deploy/` | Dockerfile, unidad systemd, configuración nginx y [`DEPLOY.md`](deploy/DEPLOY.md) |

## ISA

La CPU implementa un subconjunto de x86 de 32 bits codificado como código máquina. Cualquier otro opcode devuelve el error "opcode not implemented".

| Instrucción | Codificación |
|---|---|
| `MOV r32, imm32` | `B8+rd` |
| `MOV r/m32, r32`, `ADD`, `SUB`, `XOR`, `CMP` (registro a registro) | `89`, `01`, `29`, `31`, `39` |
| `ADD`, `AND`, `SUB`, `XOR`, `CMP` con `imm32` | `81 /0 /4 /5 /6 /7` |
| `MOV EAX, [dir32]` / `MOV [dir32], EAX` | `A1` / `A3` |
| `INC r32` / `DEC r32` | `40+rd` / `48+rd` |
| `PUSH r32` / `POP r32` | `50+rd` / `58+rd` |
| `JMP rel8` / `JMP rel32` | `EB` / `E9` |
| `Jcc rel8` (conjunto completo de condiciones de Intel) | `70`-`7F` |
| `CALL rel32` / `RET` | `E8` / `C3` |
| `INT imm8` / `IRET` | `CD` / `CF` |
| `HLT`, `CLI`, `STI` (privilegiadas: `#GP` en PL3) | `F4`, `FA`, `FB` |
| `NOP` | `90` |

Registros visibles: `EAX EBX ECX EDX ESI EDI ESP EBP EIP EFLAGS CS DS SS`. Banderas modeladas: CF, PF, ZF, SF, IF y OF. El direccionamiento se limita a las formas registro a registro y a las formas absolutas `A1`/`A3`; no hay SIB, desplazamientos ni prefijos de segmento. Tampoco hay paginación, TSS, consulta de GDT ni PIC.

Hay dos modos: **PL3 (usuario)** y **PL0 (kernel)**.

### Vectores de la IDT

| Vector | Uso |
|---|---|
| `0x00` | Error de división |
| `0x0D` | Protección general (`HLT`, `CLI` o `STI` en PL3, o `INT` a un vector que el código de usuario no puede invocar) |
| `0x0E` | Fallo de página |
| `0x20` | Timer (IRQ 0) |
| `0x21` | Teclado (IRQ 1) |
| `0x80` | Syscalls (convención de Linux i386: `EAX` = número, `EBX`/`ECX`/`EDX` = argumentos) |
| `0x81` | Sincronización (operaciones de mutex y semáforo) |

Desde modo usuario solo se permiten `INT 0x80` e `INT 0x81`.

### Syscalls

El kernel implementa `exit` (1), `fork` (2), `read` (3), `write` (4), `open` (5), `waitpid` (7), `mknod` (14), `pipe` (42), `clone` (120) y `vfork` (190). Un número desconocido devuelve `0xFFFFFFFF` en `EAX`, equivalente didáctico de `-ENOSYS`.

## Memoria

El espacio físico es de 4 MB, dividido en regiones cuyos permisos se verifican contra el modo actual de la CPU.

| Rango | Región | Usuario (PL3) | Kernel (PL0) |
|---|---|---|---|
| `0x000000-0x0FFFFF` | reservada | ninguno | R/W |
| `0x100000-0x1FFFFF` | kernel: IDT en `0x100000`, manejadores desde `0x101000`, tope de la pila en `0x1FFFF0` | ninguno | R/W/X |
| `0x200000-0x2FFFFF` | código y datos de usuario | R/X | R/W/X |
| `0x300000-0x3FFFFF` | pila de usuario | R/W | R/W |

Un acceso que viole estos permisos produce un fallo de página (vector `0x0E`) y el kernel termina el proceso infractor.

## Demos

Las 30 demos se agrupan en ocho categorías.

| Categoría | Demos |
|---|---|
| Fundamentos de CPU | `add`, `counter`, `pushpop`, `callret` |
| Modo dual y syscalls | `soft_int`, `hello`, `readkey` |
| Planificación | `idle_demo`, `timer_loop`, `two_counters`, `scheduler_demo`, `latency_quantum` |
| Procesos e IPC | `demo_fork`, `demo_vfork`, `demo_pipe` |
| Hilos | `threads_join`, `threads_race`, `threads_safe` |
| Sincronización | `race_counter`, `safe_counter`, `producer_consumer`, `philos_deadlock`, `philos_safe` |
| Fallos y excepciones | `badmem`, `stack_overflow`, `syscall_kernel_pointer`, `unknown_syscall`, `double_unlock`, `semaphore_leak` |
| Muestra completa | `showcase` (cuatro núcleos) |

Escenarios canónicos:

1. **Timer y cambio de contexto**: `two_counters` (P1 incrementa `EAX`, P2 incrementa `EBX`, el planificador round-robin los alterna en cada quantum).
2. **Syscall**: `hello` (`sys_write` de "hola SO" y `sys_exit`) y `soft_int` (un `int 0x80` manual).
3. **Fallo de página**: `badmem` (escribe en una dirección del kernel desde PL3, lo que dispara el vector `0x0E`).
4. **IRQ de teclado y bloqueo**: `readkey` (`sys_read` bloquea hasta que el usuario escribe en la consola).
5. **Condición de carrera y exclusión mutua**: `race_counter` frente a `safe_counter`, y `threads_race` frente a `threads_safe`.
6. **Interbloqueo**: `philos_deadlock` frente a `philos_safe`.

## Convenciones del simulador

- **La CPU solo cambia de modo desde el hardware**: en `INT`, `IRET`, IRQ y fallos. El software nunca escribe el modo.
- **Cada `Step()` emite eventos tipados**: la CPU devuelve `[]cpu.Event` por instrucción. Toda la interfaz y la bitácora se reconstruyen a partir de ese flujo.
- **`pkg/cpu` no conoce el planificador ni las syscalls**: son decisiones de software que viven en `pkg/kernel`.

## Inicio rápido

Requisitos: Go 1.26 o superior y Node.js (el Dockerfile compila con Node 24).

**Desarrollo.** Se ejecutan el backend y el servidor de desarrollo del frontend en dos terminales:

```bash
make dev        # backend Go en :9080
make web-install
make web-dev    # Vite en :5173, redirige /api y /ws a :9080
```

Se abre http://localhost:5173. `make watch` ejecuta ambos con recarga automática (instala [air](https://github.com/air-verse/air) si no está presente).

**Binario único.** El binario de Go embebe el frontend compilado desde `internal/staticui/dist` mediante una directiva `go:embed`, y ese directorio no se versiona. Debe compilarse primero el frontend; `make build` lo hace una vez instaladas las dependencias de npm:

```bash
make web-install build
./bin/modos-cpu                       # escucha en :9080
./bin/modos-cpu -addr 127.0.0.1:8080  # dirección personalizada
```

Un `go build ./cmd/modos-cpu` directo falla si `internal/staticui/dist` no existe. Para trabajar solo en el backend se puede omitir Node con un directorio vacío; el servidor muestra entonces una página provisional en lugar de la interfaz:

```bash
mkdir -p internal/staticui/dist && touch internal/staticui/dist/.keep
go test ./...
```

**Pruebas y verificaciones.**

```bash
make test        # go test ./...
make lint        # golangci-lint
make cover       # cobertura
make fuzz        # fuzzing de la CPU durante 15 segundos
cd web && npm test
```

**Uso de la CLI.** El binario ejecuta una demo sin servidor ni frontend:

```bash
./bin/modos-cpu cli -program counter -trace
./bin/modos-cpu cli -program two_counters -quantum 4 -trace
```

| Opción | Valor por defecto | Significado |
|---|---|---|
| `-program` | `counter` | Nombre de la demo. Un nombre desconocido imprime la lista de demos. |
| `-trace` | desactivado | Imprime cada evento emitido. |
| `-max-steps` | `200` | Máximo de instrucciones a ejecutar. |
| `-quantum` | `8` | Ticks entre interrupciones de timer (`0` desactiva el timer). |

Otros comandos:

```bash
./bin/modos-cpu admin-hash -pass='su-contraseña'   # hash bcrypt para ADMIN_PASS_HASH
./bin/modos-cpu -addr :8080                        # servidor (por defecto :9080)
```

## Configuración

La configuración se lee de variables de entorno. El servidor también carga al arrancar un archivo `.env` del directorio de trabajo, sin sobrescribir las variables que ya estén definidas. [`.env.sample`](.env.sample) es la referencia de las variables disponibles, sus valores por defecto y su significado.

Mínimo para ejecutar en local: nada. Sin `.env`, el servidor escucha en `:9080`, acepta los orígenes del servidor de desarrollo de Vite, guarda su estado en `$XDG_STATE_HOME/modos-cpu/state.db` (o `~/.local/state/modos-cpu/state.db`) y mantiene desactivado el panel de administración. La ruta de la base de datos se cambia con `MODOS_STATE_DB`.

Para activar el panel de administración deben definirse las tres variables siguientes a la vez; si falta alguna, `/admin` devuelve el mismo 404 que cualquier ruta desconocida:

```bash
make admin-hash PASS='su-contraseña'   # imprime el hash bcrypt
make session-secret                    # imprime 32 bytes aleatorios en hexadecimal (requiere openssl)
```

```env
ADMIN_USER=admin
ADMIN_PASS_HASH=<hash bcrypt>
SESSION_SECRET=<hex>
```

`SESSION_SECRET` también deriva la clave con que se cifran en la base de datos las claves de API y las contraseñas SMTP, y sirve de sal para el hash de las direcciones IP de los clientes; por ello debe definirse en cualquier despliegue real. Detrás de un dominio real debe definirse además `ALLOWED_ORIGINS` (lista de orígenes permitidos para el WebSocket) y, si hay un proxy inverso delante, `TRUSTED_PROXY_HEADER` (la cabecera con la IP real del cliente). La lista completa de medidas de endurecimiento está en [SECURITY.md](SECURITY.md).

**Variables del tutor.** También pueden definirse desde el panel de administración, que las guarda en la base de datos con las claves de API cifradas.

| Variable | Propósito |
|---|---|
| `LLM_PROVIDER` | `gemini` (por defecto), `deepseek`, `groq` u `openai_compat` |
| `LLM_MODEL` | Sustituye el modelo por defecto del proveedor |
| `GEMINI_API_KEY`, `DEEPSEEK_API_KEY`, `GROQ_API_KEY` | Clave de API del proveedor elegido |
| `LLM_API_KEY`, `LLM_BASE_URL` | Para `openai_compat`: la clave y la URL completa del endpoint de chat completions |
| `EMBEDDER_PROVIDER` | `ollama` o `jina` |
| `OLLAMA_HOST`, `OLLAMA_EMBED_MODEL` | Dirección de Ollama (por defecto `http://localhost:11434`) y modelo (por defecto `bge-m3`) |
| `JINA_API_KEY` | Clave para el embedder `jina` (modelo `jina-embeddings-v3`) |

**Indexación del corpus.** `modos-rag-index` debe usar el mismo proveedor y modelo de embeddings que usará el servidor para las consultas:

```bash
export EMBEDDER_PROVIDER=ollama     # o jina, con JINA_API_KEY definida
go run ./cmd/modos-rag-index -source all
```

| Opción | Valor por defecto | Significado |
|---|---|---|
| `-source` | `glossary` | `glossary`, `pdf`, `demos`, `isa`, `docs` o `all` |
| `-db` | ruta por defecto del servidor | Archivo SQLite donde se escribe |
| `-pdf` | ninguno | PDF a indexar (obligatorio con `-source pdf`; opcional con `all`) |
| `-pdf-source-id` | `silberschatz-9e-es` | Identificador corto del libro usado en las citas |
| `-chunk-tokens`, `-chunk-overlap` | `450`, `0.10` | Tamaño y solapamiento de los fragmentos de PDF |
| `-glossary` | `web/src/data/glossary.ts` | Archivo del glosario |
| `-repo-root` | `.` | Raíz del repositorio, para `docs` |
| `-version` | automática | Versión del corpus a escribir (la anterior más uno) |
| `-purge` | desactivado | Borra primero las filas de esa versión |
| `-batch` | `16` | Tamaño de lote del embedder |
| `-timeout` | `10m` | Límite total de tiempo |
| `-rate-limit-retries`, `-rate-limit-wait` | `5`, `65s` | Política de reintentos cuando el embedder limita la tasa |

## Tutor (ARCHIUS)

ARCHIUS es un tutor opcional que responde preguntas de los estudiantes mediante generación aumentada por recuperación (RAG): busca en un corpus indexado los pasajes relevantes y los entrega a un modelo de lenguaje junto con la pregunta. Lee una vista compacta de la sesión en vivo (sección actual, programa, modo, PID, número de núcleos, tick, `EIP`, `ESP`, `EFLAGS`, colas de listos y de espera, quantum del timer) y puede consultar más estado bajo demanda: procesos, eventos recientes, salida de la consola, estadísticas del planificador, código fuente del programa y memoria. Responde en modo directo o en modo socrático, según lo que el administrador seleccione.

Para funcionar necesita:

1. **Un LLM de chat** con clave de API. Proveedores admitidos: `gemini` (por defecto), `deepseek`, `groq` y `openai_compat` para cualquier endpoint de chat completions compatible con OpenAI.
2. **Un proveedor de embeddings** para la recuperación: `ollama` (por defecto, se ejecuta en local) o `jina` (alojado).
3. **Un corpus indexado** en la base de datos SQLite.

Si falta alguno de estos elementos, el servidor registra que el tutor está desactivado y sigue funcionando. El resto del simulador no depende del tutor.

### Corpus propio

El corpus de libros de texto que usa la demo pública está protegido por derechos de autor y no se distribuye con este repositorio. Cada instalación construye el suyo con `modos-rag-index` (las opciones están en la sección Configuración). Con `-source all` y sin `-pdf` se indexan el glosario, las descripciones de las demos, la referencia de la ISA y la documentación del proyecto, lo que no requiere material externo. Para añadir un libro, se pasa con `-pdf` un PDF cuyo uso esté autorizado. Tras indexar, se reinicia el servidor o se recarga el tutor desde el panel de administración.

### Datos enviados a terceros

Con el tutor activo, cada pregunta se envía al proveedor de LLM configurado junto con los turnos recientes de la conversación (hasta 10; 3 por defecto), los pasajes recuperados del corpus, la vista compacta de la sesión y los resultados de las consultas de estado que haga el modelo. Con el embedder `jina`, el texto de la pregunta también se envía a Jina; con `ollama` permanece en el equipo configurado. Con el tutor desactivado, este no envía nada a terceros.

### Datos almacenados localmente

Los eventos de uso (tipo, nombre, duración e identificadores de sesión y de usuario) y las preguntas y respuestas del tutor (con citas, latencia, proveedor y modelo) se guardan en la base de datos SQLite local. Sirven de base para las métricas del panel de administración. Quien despliegue el sistema para estudiantes debe informarles qué se registra.

## Deploy

[`deploy/DEPLOY.md`](deploy/DEPLOY.md) cubre tres opciones: una imagen Docker (compilación en varias etapas, entorno de ejecución distroless), un binario estático bajo systemd y un proxy inverso nginx con reenvío de WebSocket. El backend no termina TLS: debe ejecutarse detrás de un proxy inverso que lo haga. Por defecto la base de datos se guarda en el directorio de estado del usuario, así que en un contenedor conviene montar un volumen persistente y apuntar `MODOS_STATE_DB` a él.

## Seguridad

[SECURITY.md](SECURITY.md) describe las versiones con soporte, cómo reportar una vulnerabilidad de forma privada y una lista de endurecimiento para quienes alojan su propia instancia.

## Licencia

MIT. Véase [LICENSE](LICENSE).
