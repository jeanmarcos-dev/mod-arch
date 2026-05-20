// modos-cpu: didactic OS visualizer on a simplified x86.
//
// It has two modes:
//
//	modos-cpu                          # starts HTTP+WS server
//	modos-cpu cli -program counter    # runs a program via CLI with trace
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jeanmarcos/modos-cpu/internal/envfile"
	"github.com/jeanmarcos/modos-cpu/internal/programs"
	"github.com/jeanmarcos/modos-cpu/internal/server"
	"github.com/jeanmarcos/modos-cpu/internal/sessions"
	"github.com/jeanmarcos/modos-cpu/internal/sim"
	"github.com/jeanmarcos/modos-cpu/internal/staticui"
	"github.com/jeanmarcos/modos-cpu/internal/store"
	"github.com/jeanmarcos/modos-cpu/pkg/cpu"
	"github.com/jeanmarcos/modos-cpu/pkg/kernel"
)

func main() {
	if err := envfile.Load(".env"); err != nil {
		fmt.Fprintf(os.Stderr, "warn: load .env failed: %v\n", err)
	}
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "cli":
			runCLI(os.Args[2:])
			return
		case "admin-hash":
			runAdminHash(os.Args[2:])
			return
		}
	}
	runServer(os.Args[1:])
}

// runAdminHash generates the bcrypt hash for the container's ADMIN_PASS_HASH
// env var.
//
//	modos-cpu admin-hash -pass=micontraseña
func runAdminHash(args []string) {
	fs := flag.NewFlagSet("admin-hash", flag.ExitOnError)
	pass := fs.String("pass", "", "contraseña en texto plano")
	_ = fs.Parse(args)
	if *pass == "" {
		fmt.Fprintln(os.Stderr, "uso: modos-cpu admin-hash -pass=micontraseña")
		os.Exit(2)
	}
	h, err := server.HashAdminPassword(*pass)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(h)
}

func runServer(args []string) {
	fs := flag.NewFlagSet("modos-cpu", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "dirección donde escuchar")
	_ = fs.Parse(args)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	demos := make([]sim.Demo, 0)
	for _, d := range programs.Demos() {
		procs := make([]sim.Proc, 0, len(d.Procs))
		for _, p := range d.Procs {
			procs = append(procs, sim.Proc{Name: p.Name, Bytes: p.Bytes, Source: p.Source})
		}
		demos = append(demos, sim.Demo{
			Name:        d.Name,
			Description: d.Description,
			Category:    d.Category,
			Procs:       procs,
		})
	}

	files, err := staticui.Files()
	if err != nil {
		logger.Warn("embedded frontend unavailable, serving placeholder", "err", err)
	}

	dbPath, err := store.DefaultPath()
	if err != nil {
		logger.Error("cannot resolve state db path", "err", err)
		os.Exit(1)
	}
	st, err := store.Open(dbPath)
	if err != nil {
		logger.Error("cannot open state db", "path", dbPath, "err", err)
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()
	logger.Info("state db ready", "path", dbPath)

	numCores := envInt("MODOS_CPU_CORES", 1)
	manager := sessions.NewManager(sessions.Config{
		Demos:         demos,
		Logger:        logger,
		Store:         st,
		MaxPublic:     envInt("MAX_PUBLIC_SESSIONS", 10),
		MaxPrivileged: envInt("MAX_PRIVILEGED_SESSIONS", 5),
		IdleTTL:       envDuration("SESSION_TTL", 30*time.Minute),
		InviteTTL:     envDuration("INVITE_TTL", 24*time.Hour),
		NumCores:      numCores,
	})
	logger.Info("cpu cores configured", "num_cores", numCores)

	s := server.New(server.Config{
		Manager:  manager,
		Logger:   logger,
		StaticFS: files,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Background cleanup removes idle sessions and expired invites on each tick.
	manager.StartCleanup(ctx, envDuration("CLEANUP_INTERVAL", time.Minute))

	if err := s.ListenAndServe(ctx, *addr); err != nil {
		logger.Error("server stopped with error", "err", err)
		os.Exit(1)
	}
}

//nolint:gocognit // CLI flag parsing + subcommand dispatch; flat by design.
func runCLI(args []string) {
	fs := flag.NewFlagSet("modos-cpu cli", flag.ExitOnError)
	progName := fs.String("program", "counter", "demo a ejecutar (add, counter, pushpop, callret, soft_int, timer_loop, two_counters)")
	trace := fs.Bool("trace", false, "imprime cada evento emitido")
	maxSteps := fs.Int("max-steps", 200, "tope de instrucciones a ejecutar")
	quantum := fs.Int("quantum", 8, "ticks entre fires del timer (0 apaga el timer)")
	_ = fs.Parse(args)

	var demo *programs.Demo
	for _, d := range programs.Demos() {
		if d.Name == *progName {
			dd := d
			demo = &dd
			break
		}
	}
	if demo == nil {
		fmt.Fprintf(os.Stderr, "demo desconocido: %q\n", *progName)
		fmt.Fprintln(os.Stderr, "disponibles:")
		for _, d := range programs.Demos() {
			fmt.Fprintf(os.Stderr, "  - %-14s %s\n", d.Name, d.Description)
		}
		os.Exit(2)
	}

	k := kernel.New(cpu.DefaultMemSize)
	k.TimerQuantum = *quantum
	for _, p := range demo.Procs {
		pid, err := k.AddProcess(p.Name, p.Bytes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "AddProcess(%s): %v\n", p.Name, err)
			os.Exit(1)
		}
		fmt.Printf("▸ PID=%d  %-16s  %d bytes @ 0x%06X\n", pid, p.Name, len(p.Bytes), 0x200000+(pid-1)*0x40000)
	}
	fmt.Printf("▸ %s — quantum=%d · %d procesos\n", demo.Name, *quantum, len(demo.Procs))
	fmt.Printf("  %s\n\n", demo.Description)

	bootCPU := k.BootCPU()
	var consoleOut []byte
	for step := 0; step < *maxSteps && !bootCPU.Halted; step++ {
		evs, err := k.Step()
		if err != nil {
			fmt.Fprintf(os.Stderr, "step %d error: %v\n", step, err)
			os.Exit(1)
		}
		for _, e := range evs {
			if c, ok := e.(cpu.EventConsoleOut); ok {
				consoleOut = append(consoleOut, c.Bytes...)
			}
		}
		if *trace {
			printStep(step, evs)
		}
	}

	if !bootCPU.Halted {
		fmt.Fprintf(os.Stderr, "\n⚠ no halted tras %d steps\n", *maxSteps)
		os.Exit(1)
	}

	fmt.Println()
	dumpRegs(bootCPU)
	fmt.Printf("\n▸ HALT @ EIP=0x%08X tras %d ticks\n", bootCPU.R.EIP, bootCPU.Tick)
	if len(consoleOut) > 0 {
		fmt.Printf("\n▸ CONSOLE OUTPUT (%d bytes):\n----------\n%s----------\n", len(consoleOut), string(consoleOut))
	}
}

//nolint:gocyclo // event-type formatter; flat switch over event variants.
func printStep(step int, evs []cpu.Event) {
	for _, e := range evs {
		switch ev := e.(type) {
		case cpu.EventInstruction:
			fmt.Printf("  [%04d] 0x%06X  %-30s  bytes=% X\n", step, ev.EIP, ev.Mnemonic, ev.Bytes)
		case cpu.EventRegWrite:
			fmt.Printf("           ↳ %s: 0x%08X → 0x%08X\n", ev.Reg, ev.Old, ev.New)
		case cpu.EventMemRead:
			fmt.Printf("           ↳ READ  [0x%08X] = 0x%X (%d bytes)\n", ev.Addr, ev.Value, ev.Size)
		case cpu.EventMemWrite:
			fmt.Printf("           ↳ WRITE [0x%08X] = 0x%X (%d bytes)\n", ev.Addr, ev.New, ev.Size)
		case cpu.EventIRQFire:
			fmt.Printf("           ↳ ◉ IRQ 0x%02X (%s)\n", ev.Vector, ev.Source)
		case cpu.EventModeSwitch:
			fmt.Printf("           ↳ ▸ MODE PL%d → PL%d (%s)\n", ev.From, ev.To, ev.Cause)
		case cpu.EventSyscall:
			fmt.Printf("           ↳ ⚙ sys_%s(%d, 0x%X, %d) → %d  [PID=%d]\n", ev.Name, ev.Args[0], ev.Args[1], ev.Args[2], int32(ev.Result), ev.PID)
		case cpu.EventConsoleOut:
			fmt.Printf("           ↳ → CONSOLE[%d]: %q\n", ev.PID, ev.Text)
		case cpu.EventPageFault:
			fmt.Printf("           ↳ ✗ PAGE_FAULT @ 0x%X (mode PL%d, needed %s)\n", ev.Addr, ev.Mode, ev.PermNeeded)
		case cpu.EventSchedDecision:
			fmt.Printf("           ↳ ⇄ sched P%d → P%d (ready=%v)\n", ev.FromPID, ev.ToPID, ev.ReadyQueue)
		case cpu.EventPCBSave:
			fmt.Printf("           ↳ ◍ PCB save P%d  (EIP=0x%X)\n", ev.PID, ev.Regs["EIP"])
		case cpu.EventPCBRestore:
			fmt.Printf("           ↳ ◌ PCB restore P%d  (EIP=0x%X)\n", ev.PID, ev.Regs["EIP"])
		case cpu.EventProcessState:
			fmt.Printf("           ↳ P%d → %s%s\n", ev.PID, ev.State, optReason(ev.Reason))
		case cpu.EventHalt:
			fmt.Printf("           ↳ HALT\n")
		}
	}
}

func optReason(r string) string {
	if r == "" {
		return ""
	}
	return " (" + r + ")"
}

func dumpRegs(c *cpu.CPU) {
	fmt.Println("┌─ CPU REGISTERS ──────────────────────┐")
	fmt.Printf("│ EAX = 0x%08X    EBX = 0x%08X │\n", c.R.EAX, c.R.EBX)
	fmt.Printf("│ ECX = 0x%08X    EDX = 0x%08X │\n", c.R.ECX, c.R.EDX)
	fmt.Printf("│ ESI = 0x%08X    EDI = 0x%08X │\n", c.R.ESI, c.R.EDI)
	fmt.Printf("│ ESP = 0x%08X    EBP = 0x%08X │\n", c.R.ESP, c.R.EBP)
	fmt.Printf("│ EIP = 0x%08X    EFLAGS = %s  │\n", c.R.EIP, flagsString(c.R.EFLAGS))
	fmt.Printf("│ MODE = PL%d (%s)                       │\n", c.Mode, modeName(c.Mode))
	fmt.Println("└──────────────────────────────────────┘")
}

func modeName(m uint8) string {
	if m == cpu.ModeKernel {
		return "kernel"
	}
	return "user"
}

// envInt reads an env var as int or returns the default. Logs to stderr when
// the value is set but unparseable so deploys do not fail silently.
func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: env %s=%q no es int, uso default %d\n", key, v, def)
		return def
	}
	return n
}

// envDuration reads an env var as time.Duration ("30m", "1h", ...) or returns
// the default.
func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: env %s=%q no es duration válida, uso default %s\n", key, v, def)
		return def
	}
	return d
}

func flagsString(f uint32) string {
	s := []byte("------")
	if f&cpu.FlagCF != 0 {
		s[0] = 'C'
	}
	if f&cpu.FlagZF != 0 {
		s[1] = 'Z'
	}
	if f&cpu.FlagSF != 0 {
		s[2] = 'S'
	}
	if f&cpu.FlagIF != 0 {
		s[3] = 'I'
	}
	if f&cpu.FlagOF != 0 {
		s[4] = 'O'
	}
	return string(s)
}
