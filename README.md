# MODARCH

Visualizador didáctico de sistemas operativos sobre una arquitectura x86 simplificada. Ejecuta programas hand-encoded paso a paso y muestra en vivo lo que un sistema operativo hace sobre la CPU: cambios de modo kernel/user, interrupciones de hardware (timer, teclado), syscalls, preservación de PCB en context switches, y violaciones de memoria.

Pensado como apoyo para una materia de Sistemas Operativos.

## Stack

- **Backend**: Go (binario único que embebe el frontend vía `embed.FS`).
- **Frontend**: Vue 3 + Vite + TypeScript + Pinia.
- **Comunicación**: WebSocket — el backend emite eventos atómicos del CPU; el frontend reconstruye el estado a partir del stream.
- **Estética**: terminal CRT (ámbar, scanlines, JetBrains Mono / Major Mono Display).

## Estructura del repositorio

| Path | Qué hay |
|---|---|
| `pkg/cpu/` | Simulador del CPU: registros, memoria perm-checked, ISA, IDT, modos PL3/PL0 |
| `pkg/kernel/` | Software del SO: PCB, scheduler RR, syscalls, handlers de IRQ |
| `internal/sim/` | Orquestador que envuelve `pkg/kernel` y emite eventos al WS |
| `internal/sessions/` | Multiplexor por sesión (cookie) con cap y cleanup |
| `internal/server/` | HTTP + WebSocket + admin |
| `internal/server/templates/` | HTML/CSS/JS embebido para placeholder, full, admin login y admin dashboard |
| `internal/programs/` | Demos hand-encoded (`counter`, `hello`, `badmem`, `readkey`, `two_counters`, …) |
| `internal/staticui/` | Embebido del `web/dist` compilado |
| `cmd/modos-cpu/` | Punto de entrada: server, CLI runner y `admin-hash` |
| `web/` | Frontend Vue 3 (CRT theme) |
| `deploy/` | Dockerfile + systemd unit + nginx reverse proxy + runbook de Proxmox |

## ISA

Subconjunto de x86 32-bit (~25 opcodes): `MOV`, `PUSH`, `POP`, `ADD`, `SUB`, `INC`, `DEC`, `CMP`, `XOR`, `JE`/`JNE`, `JMP`, `CALL`/`RET`, `INT imm8`, `IRET`, `HLT`.

13 registros visibles: `EAX EBX ECX EDX ESI EDI ESP EBP EIP EFLAGS CS DS SS`.

Dos modos: **PL3 (user)** y **PL0 (kernel)**. IDT con cinco vectores relevantes:

| Vector | Uso |
|---|---|
| `0x00` | Divide error |
| `0x0E` | Page fault |
| `0x20` | Timer (IRQ 0) |
| `0x21` | Keyboard (IRQ 1) |
| `0x80` | Syscalls (convención Linux i386: `EAX`=nº, `EBX/ECX/EDX`=args) |

## Memoria

Espacio físico de 4 MB con regiones perm-checked contra el modo actual del CPU.

| Rango | Región | Permisos |
|---|---|---|
| `0x000000-0x0FFFFF` | BIOS / IDT / GDT | reservado |
| `0x100000-0x1FFFFF` | kernel `.text` + `.data` + stack | PL0 R/W/X |
| `0x200000-0x2FFFFF` | user `.text` + `.data` | PL3 R/X, PL0 R/W/X |
| `0x300000-0x3FFFFF` | user stack | PL3 R/W, PL0 R/W |

## Demos pedagógicos

Cuatro escenarios canónicos cubiertos por los demos embebidos:

1. **Timer + context switch** — `two_counters` (P1 incrementa `EAX`, P2 incrementa `EBX`, RR a cada quantum).
2. **Syscall** — `hello` (`sys_write "hola SO"` + `sys_exit`) y `soft_int` (manual `int 0x80`).
3. **Page fault** — `badmem` (escribe a una dirección kernel desde PL3 → IRQ 0x0E → animación shake en el frontend).
4. **Keyboard IRQ + bloqueo** — `readkey` (`sys_read` bloquea hasta que el usuario tipea en el input del ConsolePanel).

## Quickstart

```bash
# Backend en modo dev
make dev

# Frontend en modo dev (otro shell)
make web-dev

# Tests Go
make test

# Lint
make lint

# Coverage
make cover

# Binario único con frontend embebido
make web-install web-build build
./bin/modos-cpu --addr 127.0.0.1:8080
```

## Run vía CLI (sin servidor)

```bash
./bin/modos-cpu cli -program counter -trace
./bin/modos-cpu cli -program two_counters -quantum 4 -trace
```

## Multi-sesión y admin

Cada cliente recibe una cookie `mc_sid` y obtiene su propio simulador aislado. El cap por defecto es 10 sesiones públicas + 5 privilegiadas. Cuando se llena, `/` sirve la página `/full` con la lista de sesiones compartidas a las que el visitante puede unirse como espectador (cookie `mc_view`).

El panel de admin en `/admin` está deshabilitado por defecto y se activa al definir las tres variables de entorno:

```bash
make admin-hash PASS='your-password'   # imprime el hash bcrypt
make session-secret                    # genera 32 bytes hex
```

```env
ADMIN_USER=admin
ADMIN_PASS_HASH=$2a$10$...
SESSION_SECRET=<hex>
```

Desde el dashboard se puede ver las sesiones activas, matarlas, editar el alias, marcar público/privado y generar invites privilegiados (URLs `/?t=<token>` que bypasean el cap público).

## Compartir una sesión (modo presentador)

El dueño de una sesión puede activar el compartido desde el TopBar. Eso genera una URL `/v/<token>` que entra en modo viewer (read-only): el espectador ve los mismos eventos en tiempo real pero no puede ejecutar comandos.

## Convenciones del simulador

- **El CPU sólo cambia de modo desde HW**: en `INT`, `IRET`, IRQ y faults. Software nunca toca el bit.
- **Cada `Step()` emite eventos tipados**: el CPU devuelve `[]cpu.Event` por instrucción. Toda UI / log se reconstruye del stream.
- **`Step()` es determinista**: dado el mismo estado inicial, la secuencia de eventos es idéntica. Habilita replay didáctico.
- **`pkg/cpu/` no sabe del scheduler ni de syscalls**: esas son decisiones SW que viven en `pkg/kernel/`.

## Deploy

Ver [`deploy/DEPLOY.md`](deploy/DEPLOY.md) para el runbook completo. Incluye Docker (imagen distroless ~10 MB), binario + systemd, y reverse proxy nginx con WebSocket forwarding.

## License

MIT.
