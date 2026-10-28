# MOD-ARCH

[Español](README.es.md)

MOD-ARCH is an educational simulator for teaching operating systems on a simplified 32-bit x86 machine. It runs hand-encoded programs step by step and shows, live, what an operating system does on the CPU: kernel/user mode switches, hardware interrupts (timer, keyboard), syscalls, process control block (PCB) preservation across context switches, scheduling, synchronization and memory violations. The backend is written in Go and the frontend in Vue 3. It ships with 30 curricular demos, a read-only sharing mode for classrooms, an admin panel, a UI in Spanish, English and Portuguese, and an optional retrieval-augmented tutor (ARCHIUS).

**Live demo:** https://mod-arch-simulator.com

## Paper and recognition

MOD-ARCH was presented at the XXI Latin American Conference on Learning Technologies (LACLO 2026) in Rio de Janeiro:

- J. Juárez, A. Morales Bezeira, A. Silva Sprock. *MOD-ARCH 1.0: An Open-Concept Educational Tool for Teaching Operating Systems*. LACLO 2026. To appear in Springer Lecture Notes in Educational Technology.
- Third place in the LACLO 2026 Learning Objects Competition with *MOD-ARCH: A Laboratory for Operating Systems Practical Sessions* ([official list of awarded works](https://lacloconference.com/awarded-papers/)).

## Authors

- Jeanmarcos Juárez
- Ana Morales Bezeira
- Antonio Silva Sprock

## Features

- **CPU**: a subset of 32-bit x86 with two privilege levels (PL0 kernel, PL3 user), an IDT, permission-checked memory regions and a typed event for every observable effect of an instruction. One to four simulated cores.
- **Kernel**: PCBs, a round-robin scheduler driven by a configurable timer quantum, `fork`, `vfork`, `clone`, `waitpid`, pipes, mutexes and semaphores, blocking `read`, and an optional idle task.
- **Demos**: 30 hand-encoded programs, each with its C-like source, a source-to-instruction map and a variables panel (see [Demos](#demos)).
- **Visualization**: live views for processes, memory, interrupts and concurrency, an event log, a built-in glossary, four color themes, three presentation modes (compact, pedagogic, projector) and an accessibility menu.
- **Languages**: Spanish, English and Portuguese (`web/src/i18n/locales/`).
- **Sessions**: every visitor gets an isolated simulator. Capacity and idle timeout are configurable.
- **Sharing**: a session owner can generate a read-only `/v/<token>` link. Spectators see the same events in real time but cannot run commands. When the public session cap is reached, `/full` lists the shared sessions that can be joined as a spectator. An optional password gate for viewer links is available from the admin panel.
- **Optional student login**: name, email and student ID, with an emailed magic link when SMTP is configured, an email allowlist and an optional Cloudflare Turnstile check.
- **Admin panel** (disabled by default): live and historical sessions, kill/label/publish sessions, users and roles (student, moderator, admin), privileged invite links, access mode (open, invite-only, closed), per-demo enable/disable, scheduled UI windows, default UI settings, SMTP settings, audit log, user reports, session history, usage and tutor metrics, and tutor configuration.
- **Tutor (ARCHIUS)**: optional, see [Tutor](#tutor-archius). The simulator works fully without it.

## Architecture and stack

- **Backend**: Go. A single binary that embeds the built frontend (`internal/staticui`). SQLite through the pure-Go `modernc.org/sqlite` driver, so builds work with `CGO_ENABLED=0`.
- **Frontend**: Vue 3, Vite, TypeScript, Pinia, vue-router and vue-i18n.
- **Transport**: WebSocket. The backend emits atomic CPU events; the frontend rebuilds its state from the stream.
- **Persistence**: a single SQLite file for users, settings, session history, usage events and the tutor corpus.

Simulator conventions:

- The CPU changes mode only from hardware: on `INT`, `IRET`, IRQs and faults. Software never writes the mode.
- Every `Step()` returns typed events (`[]cpu.Event`). All UI and logs are rebuilt from that stream.
- `pkg/cpu` knows nothing about the scheduler or syscalls. Those are software decisions that live in `pkg/kernel`.

### ISA

Instructions implemented by `pkg/cpu` (x86 machine code; anything else returns "opcode not implemented"):

| Instruction | Encoding |
|---|---|
| `MOV r32, imm32` | `B8+rd` |
| `MOV r/m32, r32`, `ADD`, `SUB`, `XOR`, `CMP` (register to register) | `89`, `01`, `29`, `31`, `39` |
| `ADD`, `AND`, `SUB`, `XOR`, `CMP` with `imm32` | `81 /0 /4 /5 /6 /7` |
| `MOV EAX, [addr32]` / `MOV [addr32], EAX` | `A1` / `A3` |
| `INC r32` / `DEC r32` | `40+rd` / `48+rd` |
| `PUSH r32` / `POP r32` | `50+rd` / `58+rd` |
| `JMP rel8` / `JMP rel32` | `EB` / `E9` |
| `Jcc rel8` (full Intel condition set) | `70`-`7F` |
| `CALL rel32` / `RET` | `E8` / `C3` |
| `INT imm8` / `IRET` | `CD` / `CF` |
| `HLT`, `CLI`, `STI` (privileged: `#GP` in PL3) | `F4`, `FA`, `FB` |
| `NOP` | `90` |

Registers: `EAX EBX ECX EDX ESI EDI ESP EBP EIP EFLAGS CS DS SS`. Modeled flags: CF, PF, ZF, SF, IF, OF. Addressing is limited to register-to-register forms and the absolute `A1`/`A3` forms; there is no SIB, displacement or segment override. There is no paging, TSS, GDT lookup or PIC (see `pkg/cpu/doc.go`).

IDT vectors:

| Vector | Use |
|---|---|
| `0x00` | Divide error |
| `0x0D` | General protection (`HLT`/`CLI`/`STI` in PL3, `INT` to a vector user code may not call) |
| `0x0E` | Page fault |
| `0x20` | Timer (IRQ 0) |
| `0x21` | Keyboard (IRQ 1) |
| `0x80` | Syscalls (Linux i386 convention: `EAX` = number, `EBX`/`ECX`/`EDX` = arguments) |
| `0x81` | Synchronization (mutex and semaphore operations) |

Only `INT 0x80` and `INT 0x81` are allowed from user mode.

Syscalls (`pkg/kernel/syscall.go`, `pkg/kernel/clone.go`): `exit` (1), `fork` (2), `read` (3), `write` (4), `open` (5), `waitpid` (7), `mknod` (14), `pipe` (42), `clone` (120) and `vfork` (190). Unknown numbers return `0xFFFFFFFF` in `EAX`.

### Memory

Physical memory is 4 MB, split into regions whose permissions are checked against the current CPU mode:

| Range | Region | User (PL3) | Kernel (PL0) |
|---|---|---|---|
| `0x000000-0x0FFFFF` | reserved | none | R/W |
| `0x100000-0x1FFFFF` | kernel: IDT at `0x100000`, handlers from `0x101000`, stack top at `0x1FFFF0` | none | R/W/X |
| `0x200000-0x2FFFFF` | user code and data | R/X | R/W/X |
| `0x300000-0x3FFFFF` | user stack | R/W | R/W |

### Demos

The 30 demos are grouped into eight categories (`internal/programs/categories.go`).

| Category | Demos |
|---|---|
| CPU fundamentals | `add`, `counter`, `pushpop`, `callret` |
| Dual mode and syscalls | `soft_int`, `hello`, `readkey` |
| Scheduling | `idle_demo`, `timer_loop`, `two_counters`, `scheduler_demo`, `latency_quantum` |
| Processes and IPC | `demo_fork`, `demo_vfork`, `demo_pipe` |
| Threads | `threads_join`, `threads_race`, `threads_safe` |
| Synchronization | `race_counter`, `safe_counter`, `producer_consumer`, `philos_deadlock`, `philos_safe` |
| Faults and exceptions | `badmem`, `stack_overflow`, `syscall_kernel_pointer`, `unknown_syscall`, `double_unlock`, `semaphore_leak` |
| Showcase | `showcase` (four cores) |

### Repository layout

| Path | Contents |
|---|---|
| `cmd/modos-cpu/` | Server entry point plus the `cli` and `admin-hash` subcommands |
| `cmd/modos-rag-index/` | Builds the tutor corpus (chunks and embeddings) inside the SQLite file |
| `cmd/ts-gen/` | Generates `web/src/types/generated.ts` from Go structs |
| `pkg/cpu/` | CPU: registers, permission-checked memory, ISA, interrupts, events |
| `pkg/kernel/` | Operating system layer: PCBs, scheduler, syscalls, IPC, synchronization |
| `internal/sim/` | Simulator that wraps the kernel and emits events to the WebSocket |
| `internal/sessions/` | Per-visitor session manager (caps, idle cleanup, sharing) |
| `internal/server/` | HTTP, WebSocket, authentication, admin API |
| `internal/programs/` | Demo catalog and a small assembler (`asm/`) |
| `internal/tutor/`, `internal/rag/`, `internal/llm/` | Tutor orchestration, retrieval and LLM clients |
| `internal/store/` | SQLite persistence (schema in `schema.sql`) |
| `internal/staticui/` | Embeds the built frontend |
| `web/` | Vue 3 frontend |
| `deploy/` | Dockerfile, systemd unit, nginx config and [`DEPLOY.md`](deploy/DEPLOY.md) |

## Quickstart

Requirements: Go 1.26 or newer and Node.js (the Dockerfile builds with Node 24).

### Development

Run the backend and the frontend dev server in two shells:

```bash
make dev        # Go backend on :9080
make web-install
make web-dev    # Vite on :5173, proxies /api and /ws to :9080
```

Open http://localhost:5173. `make watch` runs both with live reload (it installs [air](https://github.com/air-verse/air) if missing).

### Single binary

The Go binary embeds the built frontend from `internal/staticui/dist` (a `go:embed` directive), and that directory is not committed. Build the frontend first; `make build` does it for you once the npm dependencies are installed:

```bash
make web-install build
./bin/modos-cpu                       # listens on :9080
./bin/modos-cpu -addr 127.0.0.1:8080  # custom address
```

A plain `go build ./cmd/modos-cpu` fails if `internal/staticui/dist` does not exist. For backend-only work you can skip Node with an empty stub; the server then serves a placeholder page instead of the UI:

```bash
mkdir -p internal/staticui/dist && touch internal/staticui/dist/.keep
go test ./...
```

### Tests and checks

```bash
make test        # go test ./...
make lint        # golangci-lint
make cover       # coverage
make fuzz        # fuzz the CPU for 15 seconds
cd web && npm test
```

## CLI usage

The binary runs a demo headlessly, without the server or the frontend:

```bash
./bin/modos-cpu cli -program counter -trace
./bin/modos-cpu cli -program two_counters -quantum 4 -trace
```

| Flag | Default | Meaning |
|---|---|---|
| `-program` | `counter` | Demo name. An unknown name prints the list of demos. |
| `-trace` | off | Print every emitted event. |
| `-max-steps` | `200` | Maximum number of instructions to execute. |
| `-quantum` | `8` | Ticks between timer interrupts (`0` disables the timer). |

Other commands:

```bash
./bin/modos-cpu admin-hash -pass='your-password'   # bcrypt hash for ADMIN_PASS_HASH
./bin/modos-cpu -addr :8080                        # server (default :9080)
```

## Configuration

Settings are read from environment variables. The server also loads a `.env` file from the working directory at startup, without overriding variables that are already set. [`.env.sample`](.env.sample) is the reference for the available variables, their defaults and their meaning.

Minimum to run locally: nothing. Without a `.env` the server listens on `:9080`, allows the Vite dev origins, stores state in `$XDG_STATE_HOME/modos-cpu/state.db` (or `~/.local/state/modos-cpu/state.db`) and keeps the admin panel disabled. Override the database path with `MODOS_STATE_DB`.

To enable the admin panel, set these three variables together; if any is missing, `/admin` returns the same 404 as any unknown route:

```bash
make admin-hash PASS='your-password'   # prints the bcrypt hash
make session-secret                    # prints 32 random bytes as hex (needs openssl)
```

```env
ADMIN_USER=admin
ADMIN_PASS_HASH=<bcrypt hash>
SESSION_SECRET=<hex>
```

`SESSION_SECRET` also keys the at-rest encryption of API keys and SMTP passwords stored in the database and salts the hash of client IP addresses, so set it in any real deployment. Behind a real domain, also set `ALLOWED_ORIGINS` (WebSocket origin allowlist) and, when a reverse proxy sits in front, `TRUSTED_PROXY_HEADER` (the header that carries the real client IP). See [SECURITY.md](SECURITY.md) for the full hardening list.

## Tutor (ARCHIUS)

ARCHIUS is an optional tutor that answers student questions using retrieval-augmented generation (RAG). It reads a compact view of the live session (current section, program, mode, PID, core count, tick, `EIP`, `ESP`, `EFLAGS`, ready and wait queues, timer quantum) and can query more state on demand (processes, recent events, console output, scheduler statistics, program source, memory). It answers in either a direct or a Socratic mode, selected by the admin.

What it needs:

1. **A chat LLM** with an API key. Supported providers: `gemini` (default), `deepseek`, `groq`, and `openai_compat` for any OpenAI-compatible chat-completions endpoint.
2. **An embeddings provider** for retrieval: `ollama` (default, runs locally, model `bge-m3`) or `jina` (hosted, `jina-embeddings-v3`).
3. **An indexed corpus** in the SQLite database.

If any of these is missing the server logs that the tutor is disabled and keeps running. Everything else works without it.

Main variables (they can also be set from the admin panel, which stores them in the database with API keys encrypted):

| Variable | Purpose |
|---|---|
| `LLM_PROVIDER` | `gemini` (default), `deepseek`, `groq` or `openai_compat` |
| `LLM_MODEL` | Override the provider's default model |
| `GEMINI_API_KEY`, `DEEPSEEK_API_KEY`, `GROQ_API_KEY` | API key for the chosen provider |
| `LLM_API_KEY`, `LLM_BASE_URL` | For `openai_compat`: key and the full chat-completions URL |
| `EMBEDDER_PROVIDER` | `ollama` or `jina` |
| `OLLAMA_HOST`, `OLLAMA_EMBED_MODEL` | Ollama address (default `http://localhost:11434`) and model |
| `JINA_API_KEY` | Key for the `jina` embedder |

### Building your own corpus

The textbook corpus used by the public demo is copyrighted and is not distributed with this repository. You build your own with `modos-rag-index`, using the same embeddings provider and model that the server will use for queries:

```bash
export EMBEDDER_PROVIDER=ollama     # or jina, with JINA_API_KEY set
go run ./cmd/modos-rag-index -source all
```

Flags:

| Flag | Default | Meaning |
|---|---|---|
| `-source` | `glossary` | `glossary`, `pdf`, `demos`, `isa`, `docs` or `all` |
| `-db` | server default path | SQLite file to write to |
| `-pdf` | none | PDF to index (needed for `-source pdf`; optional for `all`) |
| `-pdf-source-id` | `silberschatz-9e-es` | Short book identifier used in citations |
| `-chunk-tokens`, `-chunk-overlap` | `450`, `0.10` | PDF chunk size and overlap |
| `-glossary` | `web/src/data/glossary.ts` | Glossary file |
| `-repo-root` | `.` | Repository root, for `docs` |
| `-version` | auto | Corpus version to write (the previous version plus one) |
| `-purge` | off | Delete existing rows of that version first |
| `-batch` | `16` | Embedder batch size |
| `-timeout` | `10m` | Total time limit |
| `-rate-limit-retries`, `-rate-limit-wait` | `5`, `65s` | Retry policy when the embedder is rate limited |

`-source all` without `-pdf` indexes the glossary, the demo descriptions, the ISA reference and the project docs, which needs no external material. To add a book, pass a PDF you have the right to use with `-pdf`. Restart the server (or reload the tutor from the admin panel) after indexing.

### Data sent to third parties

When the tutor is enabled, each question is sent to the configured LLM provider together with the recent conversation turns (up to 10, default 3), the retrieved corpus passages, the compact session view above and the results of any state queries the model makes. With the `jina` embedder, the question text is also sent to Jina; with `ollama` it stays on your network. The tutor sends nothing to third parties while it is disabled.

### Data stored locally

Usage events (kind, name, duration, session and user IDs) and tutor questions and answers (with citations, latency, provider and model) are stored in the local SQLite database. They back the admin metrics. If you deploy this for students, tell them what is recorded.

## Deploy

[`deploy/DEPLOY.md`](deploy/DEPLOY.md) covers three options: a Docker image (multistage build, distroless runtime), a static binary under systemd, and an nginx reverse proxy with WebSocket forwarding. The backend does not terminate TLS; run it behind a TLS-terminating reverse proxy. The database lives under the user's state directory by default, so mount a persistent volume and point `MODOS_STATE_DB` at it when running in a container.

## Security

See [SECURITY.md](SECURITY.md) for supported versions, how to report a vulnerability privately and a hardening checklist for self-hosters.

## License

MIT. See [LICENSE](LICENSE).
