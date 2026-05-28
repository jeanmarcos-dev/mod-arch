// English translations of every glossary entry. Keyed by the canonical
// Spanish `term` from glossary.ts so the lookup is a single Map.get when the
// active locale is 'en'. Identifiers (PL, EAX, IRQ, hex vectors, syscall
// names) stay in their canonical x86/Linux form regardless of locale.

export const GLOSSARY_EN: Record<string, { short: string; detail: string }> = {
  // ─── Modes ──────────────────────────────────────────────────────────────
  'PL — Privilege Level': {
    short: 'CPU privilege level. PL3 = user, PL0 = kernel.',
    detail: 'x86 defines 4 privilege levels (0-3). Conventionally PL0 = kernel (full permissions) and PL3 = user (restricted access). The intermediate levels are almost never used. The HW checks PL on every memory access, every execution of a privileged instruction, etc. SW CANNOT change the PL directly — only the HW does it on INT/IRET/exceptions.',
  },
  'RPL — Requested Privilege Level': {
    short: 'Bits 0-1 of a segment selector. Mark the privilege "requested" by that selector.',
    detail: 'Every 16-bit selector (CS, DS, SS, …) carries its RPL encoded in the two least significant bits. In user mode the selectors have RPL=3 (e.g. CS = 0x1B = 0x18 | 3); in kernel they have RPL=0 (e.g. CS = 0x08). The CPU consults the RPL on IRET to decide whether to lower its privilege: if the popped CS has RPL=3, it also pops SS and ESP from the stack and switches to user mode (inter-privilege IRET). RPL must not be confused with CPL (Current Privilege Level), which is the RPL of the CS currently in use — CPL is the "actual mode" the CPU is running; RPL is what each selector requests when loaded.',
  },
  'USER MODE (PL3)': {
    short: 'Normal processes. Restricted access.',
    detail: 'Application programs run at PL3. They cannot touch kernel memory, execute HLT, IN/OUT, modify IDT/GDT, etc. If they try, the HW raises an exception the kernel handles.',
  },
  'KERNEL MODE (PL0)': {
    short: 'Full privilege. Access to everything.',
    detail: 'OS code runs at PL0. It can read/write any memory, execute any instruction, configure HW. Entered from user via INT (syscall) or automatically when an IRQ/exception arrives.',
  },
  'Mode switch': {
    short: 'Transition between user and kernel. Performed by HW, never by SW.',
    detail: 'When an IRQ arrives or user does `int N`, the HW saves state (EFLAGS/CS/EIP) and switches mode automatically. The kernel only configures WHICH handler to invoke (via IDT); the mode change belongs to HW. On IRET, the HW pops the frame and restores the original mode.',
  },

  // ─── Registers ──────────────────────────────────────────────────────────
  'EAX, EBX, ECX, EDX': {
    short: 'General-purpose 32-bit registers.',
    detail: 'GP = General Purpose. EAX (Accumulator), EBX (Base), ECX (Counter), EDX (Data). By Linux i386 convention syscalls use: EAX = number, EBX = arg1, ECX = arg2, EDX = arg3, EAX = result.',
  },
  'ESI, EDI': {
    short: 'Source Index / Destination Index.',
    detail: 'Historically used by string instructions (MOVS, LODS, STOS) copying bytes from [ESI] to [EDI]. Today they are used as general-purpose registers.',
  },
  'ESP — Stack Pointer': {
    short: 'Points to the current top of the stack (the last occupied slot).',
    detail: 'Every PUSH decrements it by 4 bytes and writes the value; every POP reads and increments by 4. CALL/RET and INT/IRET also modify it automatically: CALL pushes the return address (RET pops it); INT pushes EFLAGS/CS/EIP (IRET pops them). The stack grows DOWNWARD — from high addresses toward low ones — so ESP decreases as work piles up. Important: the concrete value of ESP depends on the mode. In PL3 it points at the user-process stack; in PL0 it points at the kernel stack (the CPU reloads ESP automatically on a privilege change).',
  },
  'EBP — Base Pointer': {
    short: 'Points to the base of the current stack frame. A fixed anchor during a function.',
    detail: 'Unlike ESP, EBP does not move on every PUSH/POP: it stays put for the whole execution of the function so the compiler can emit predictable accesses to parameters and locals. The classic convention is the prologue `PUSH EBP; MOV EBP, ESP` on entry and `POP EBP; RET` on exit. Inside the function, arguments live at `[EBP+8]`, `[EBP+12]`, etc. (past the saved EBP and the return address), and locals at `[EBP-4]`, `[EBP-8]`, etc. In short: ESP moves constantly; EBP stays still.',
  },
  EIP: {
    short: 'Instruction Pointer. The next instruction to execute.',
    detail: 'After fetching the bytes of an instruction, EIP points to the next one. JMP/CALL/RET/INT/IRET modify it explicitly. Nobody writes EIP directly with MOV — the ISA forbids it.',
  },
  EFLAGS: {
    short: 'Flags register (status and configuration).',
    detail: 'Each bit is a distinct flag: ZF (Zero), SF (Sign), CF (Carry), OF (Overflow), IF (Interrupt enable), etc. Arithmetic/CMP instructions update the flags; conditional jumps (JE/JNE/JG/JL) read them.',
  },
  'CS, DS, SS': {
    short: 'Segment selectors (Code / Data / Stack).',
    detail: '16-bit selectors that index entries in the GDT (or LDT when TI=1; this simulator does not use LDT). Bits 0-1 are the RPL (Requested Privilege Level). In user mode CS = 0x1B (0x18 | 3); in kernel CS = 0x08. IRET reads the CS it is about to pop to decide whether to drop back to user or stay in kernel.',
  },

  // ─── Flags ──────────────────────────────────────────────────────────────
  'ZF — Zero Flag': {
    short: '1 when the last result was 0.',
    detail: 'Set by ADD/SUB/CMP/AND/etc. Read by JE/JZ (jumps if ZF=1) and JNE/JNZ (jumps if ZF=0). CMP is a SUB that only updates flags without writing the result.',
  },
  'SF — Sign Flag': {
    short: '1 when the last result was negative.',
    detail: 'Equal to the most significant bit (bit 31) of the result. Read by JG, JL, JGE, JLE for signed comparisons.',
  },
  'CF — Carry Flag': {
    short: '1 when there was an unsigned overflow.',
    detail: 'For example, adding 0xFFFFFFFF + 1 sets CF (it wraps). Used by instructions that extend arithmetic to 64 bits (ADC, SBB).',
  },
  'OF — Overflow Flag': {
    short: '1 when there was a signed overflow.',
    detail: "Distinct from CF: detects overflow IN TWO'S COMPLEMENT. E.g. 0x7FFFFFFF + 1 sets OF (it goes from max positive to min negative).",
  },
  'IF — Interrupt Flag': {
    short: 'If IF=1, HW IRQs may interrupt. If IF=0, ignored.',
    detail: 'In user mode IF is normally 1 so the timer can preempt. When an IRQ arrives, the HW pushes EFLAGS and clears IF to prevent re-entrancy. IRET restores the popped EFLAGS (re-enabling IF if it was 1). HLT with IF=1 waits for the next IRQ. Fine print: in real x86 this clear of IF depends on the IDT "gate" type — interrupt gates clear IF, trap gates do not. In this simulator every entry is treated as an interrupt gate.',
  },

  // ─── Interrupts ─────────────────────────────────────────────────────────
  Vector: {
    short: 'Index 0-255 into the IDT. Covers HW IRQs, CPU exceptions and software interrupts.',
    detail: 'Anything that enters through the IDT carries a vector that indexes the corresponding handler. The technical taxonomy distinguishes three origins for that vector: IRQ (external hardware), exception (the CPU itself when executing an instruction) and software interrupt (the INT N instruction). Colloquially — and in this simulator\'s source — "IRQ" is used as an umbrella term for the three; the glossary instead keeps the distinction so you learn real x86 terminology. Naming convention in the following entries: "Vector NN" prefix for CPU exceptions (0x00, 0x0D, 0x0E), "IRQ NN" prefix for external hardware interrupts (0x20, 0x21) and "INT NN" prefix for software interrupts (0x80, 0x81). The vector number is the same concept in all three cases: what changes is the trigger origin.',
  },
  'IRQ — Interrupt Request': {
    short: 'External signal to the CPU asking for attention.',
    detail: 'Strictly speaking, an IRQ is an asynchronous event sent to the CPU by external HW (timer, keyboard, disk). The CPU services it between instructions if IF=1. Every IRQ has a vector (number 0-255) that indexes the IDT. Exceptions (originated in the CPU) and software interrupts (INT N) share the same dispatch but are NOT IRQs in the strict sense.',
  },
  'Excepción': {
    short: "Event triggered by the CPU itself when executing a problematic instruction.",
    detail: 'External HW does not fire it: the CPU does, when an instruction cannot complete (divide by zero, invalid memory access, privileged instruction in user mode, etc.). Subclassified into fault (EIP points at the offending instruction, retryable), trap (EIP points to the next one) and abort (unrecoverable). It enters the IDT like an IRQ but technically is NOT an IRQ.',
  },
  'Software interrupt': {
    short: 'Trap triggered by the INT N instruction from code.',
    detail: 'User or kernel executes `INT N`. The HW reads IDT[N], pushes EFLAGS/CS/EIP, switches to kernel mode and jumps to the handler. Functionally identical to a HW IRQ dispatch; the difference is the origin (`source = SW` vs `HW`). Classically used for syscalls (Linux i386 uses INT 0x80). NOT an IRQ in the strict sense, even if it shares the mechanism.',
  },
  'Vector 0x00 — Divide error': {
    short: 'Exception when an integer division divides by zero (or overflows).',
    detail: 'Raised by the CPU itself when DIV or IDIV runs with a zero divisor, or when the quotient does not fit in the destination register. A fault-type exception: EIP points at the offending instruction, not the next one. The kernel usually terminates the offending process.',
  },
  'IRQ 0x20 — Timer': {
    short: 'Periodic tick of the programmable timer (PIT).',
    detail: 'Every `quantum` CPU ticks this HW IRQ fires. The scheduler uses it to preempt: save the current PCB, pick the next process, restore its PCB. This is what makes the OS preemptive multitasking. Unlike vectors 0x00 / 0x0E / 0x80 / 0x81, this one IS an IRQ in the strict sense: external HW (PIT), asynchronous.',
  },
  'IRQ 0x21 — Keyboard': {
    short: 'A key was pressed.',
    detail: 'The keyboard controller drops the scancode/byte into a buffer and fires this HW IRQ. The kernel handler reads the byte and, if there is a BLOCKED process waiting on the keyboard, hands it the byte and wakes it. Like the timer, an IRQ in the strict sense (external HW, asynchronous).',
  },
  'INT 0x80 — Syscall': {
    short: 'Software interrupt: the user requests a service from the kernel (Linux i386 convention).',
    detail: 'Not a HW IRQ: it is an `int 0x80` instruction the user executes. The HW switches to kernel mode (as with any INT), and the kernel dispatches by EAX to the corresponding syscall. Modern Linux uses `sysenter`/`syscall` for performance, but `int 0x80` still works.',
  },
  'INT 0x81 — Mutex/Semáforo': {
    short: 'Simulator-specific software interrupt for mutex and semaphore operations.',
    detail: 'Not standard Linux — a convention of this simulator. EAX picks the operation (mutex_lock=1, mutex_unlock=2, sem_wait=3, sem_signal=4); EBX carries the resource ID. The handler lives in the kernel and manipulates the mutex and semaphore lists you see at /concurrencia.',
  },
  'Vector 0x0E — Page Fault': {
    short: 'Vector of the page fault exception, raised by the MMU.',
    detail: 'The IDT slot where the MMU dispatches any invalid memory access. Like every exception, it is raised by the CPU itself (not external HW): when the memory unit detects that the access cannot be satisfied in the current mode, it reloads ESP to the kernel stack, pushes the frame and jumps to IDT[0x0E]. For the conditions that trigger it and what the kernel does next, see the "Page fault" entry in the Memory section.',
  },
  'Vector 0x0D — General Protection': {
    short: '#GP exception: user attempted something its privilege level does not allow.',
    detail: 'Raised by the CPU when a process at PL3 executes a privileged instruction (HLT, IN/OUT, LGDT/LIDT, MOV CRn, etc.) or uses an invalid selector. The kernel terminates the offending process with reason "general_protection". In this simulator, the most common cause you will see is HLT from user mode — remember that HLT is only valid at PL0.',
  },
  'INT N': {
    short: 'Instruction that triggers a software interrupt.',
    detail: 'User executes `int N`. The HW reads IDT[N], pushes EFLAGS/CS/EIP, switches to kernel mode, jumps to the handler. Functionally IDENTICAL to a HW IRQ dispatch; the difference is only the source (SW vs HW).',
  },
  IRET: {
    short: 'Return from interrupt. Undoes what INT/dispatch did.',
    detail: 'Pops from the stack, IN THIS POP ORDER: first EIP (because it is at the top), then CS, then EFLAGS. If the popped CS is user (RPL=3), it also pops ESP and SS and switches to user mode (this is called "inter-privilege IRET"). It is the only "clean" way to return from kernel to user. The physical memory of the frame, read from low to high addresses, is [EIP][CS][EFLAGS][ESP_user][SS_user]: the reverse of the pop order because the stack grows downward.',
  },
  'IDT — Interrupt Descriptor Table': {
    short: 'Table with 256 entries: vector → handler.',
    detail: 'Lives in kernel memory. Each entry points at the handler for the corresponding vector — regardless of whether that vector is fired by a HW IRQ, a CPU exception or an INT N. The HW consults it automatically on dispatch. In real x86 each entry is an 8-byte "gate descriptor"; in our simulator we simplify to 4-byte pointers.',
  },

  // ─── Processes ──────────────────────────────────────────────────────────
  'PCB — Process Control Block': {
    short: 'Kernel structure that holds the state of a process.',
    detail: 'For each process the kernel keeps a PCB with: PID, name, state, snapshot of ALL the registers, address of its code, address of its stack, list of open files, priority, etc. In Linux this is called `task_struct`.',
  },
  'PID — Process ID': {
    short: 'Unique identifier of the process.',
    detail: 'The kernel assigns a PID monotonically when creating each process. PID 1 is typically init. In our simulator, PID 1 = first process of the demo, PID 2 = second, etc.',
  },
  RUN: {
    short: 'Process currently executing on the CPU.',
    detail: 'Only ONE process at a time can be RUN (on single-core). Its registers are live in the CPU. When the timer fires, it transitions to READY.',
  },
  READY: {
    short: 'Ready to run, waiting its turn in the ready queue.',
    detail: 'The scheduler will pick it on the next preemption. Its registers live in the PCB.',
  },
  BLOCKED: {
    short: 'Waiting for an event (I/O, sleep, mutex).',
    detail: 'Does NOT consume CPU. Sits in the waiting queue. When the awaited event happens (e.g. a keyboard IRQ), the kernel moves it to READY.',
  },
  ZOMBIE: {
    short: 'Process ended but the PCB still exists.',
    detail: 'When a process calls sys_exit (or dies of an exception), the kernel marks its PCB as ZOMBIE. The PCB stays so the parent can read the exit code with wait(). Our demo keeps it for simplicity.',
  },
  'Context switch': {
    short: 'Change of the process that is in RUN.',
    detail: "When it happens: 1) Save the current process's state into its PCB. 2) Pick the next. 3) Load its PCB into the CPU. 4) Continue. In our simulator this happens between the IRQ dispatch and the IRET — the kernel-stack frame is \"patched\" with the new process's registers.",
  },
  'Ready queue': {
    short: 'FIFO queue of READY processes.',
    detail: 'RR scheduler uses it: pulls from the head to run, pushes to the tail the one that loses the CPU. Other schedulers (CFS, priorities) use more complex structures.',
  },
  'Waiting queue': {
    short: 'Queue of BLOCKED processes waiting on a resource.',
    detail: 'We render it as a single list for simplicity. In real kernels there is ONE waiting queue PER resource: one per I/O device (keyboard, disk, every network socket), one per mutex, one per semaphore, one per message queue. That way, when an event arrives — say the keyboard IRQ — the kernel knows exactly which process to wake by inspecting ONLY that resource\'s queue, without scanning the system-wide process list. The split also enables per-device policies: a disk queue can be FCFS, a socket queue can be priority-based, etc.',
  },
  'TID — Thread ID': {
    short: 'Unique identifier of a task, whether it is a process or a thread.',
    detail: 'In Linux everything is a task: what we call a "process" has its own TID, and the threads of the same process share a TGID but each one keeps its own TID. In this simulator TID matches the PID the kernel assigns to each task; the semantic alias helps distinguish "I am a thread" from "I am a process" when both share an address space.',
  },
  'TGID — Thread Group ID': {
    short: 'PID of the thread group leader. All threads of the same "process" share a TGID.',
    detail: 'fork() creates a new TGID (= child PID). clone(CLONE_THREAD) inherits the parent\'s TGID, so getpid() returns the same value for every thread of the process, while gettid() returns the real TID. In the UI we group cards by TGID so each "process" appears as a node with its threads nested underneath.',
  },
  'clone()': {
    short: 'Linux primitive syscall for creating tasks with sharing flags.',
    detail: 'fork(), vfork() and pthread_create() are particular cases of clone(). The relevant flags here: CLONE_VM (share address space), CLONE_FILES (share fd table), CLONE_THREAD (inherit TGID, become a sibling thread), CLONE_VFORK (block the parent until the child exits). Combining them yields the classic pthread_create behavior: CLONE_VM | CLONE_FILES | CLONE_THREAD.',
  },
  pthread_create: {
    short: 'POSIX API to create a thread. In glibc it translates into clone().',
    detail: 'pthread_create(thread, attr, fn, arg) asks the kernel for a task that shares memory and fds with the caller, and starts in fn(arg). In this simulator it is a wrapper over sys_clone(120) with the flags CLONE_VM|CLONE_FILES|CLONE_THREAD; pthread_join is modeled with waitpid on the TID.',
  },
  'Race condition': {
    short: 'Bug where two tasks compete for a datum without synchronization and the result depends on the interleaving.',
    detail: 'The threads_race demo shows it: two threads do counter++ which is really load-inc-store. If the timer interrupts between load and store, the other thread can read the same stale value and both end up writing the same number — one increment is lost. With a small quantum and 200 iterations the final counter sits well below 400. The didactic fix is threads_safe: bracketing the critical section with mutex_lock/unlock.',
  },

  // ─── Scheduler ──────────────────────────────────────────────────────────
  Quantum: {
    short: 'How many CPU ticks a process gets before the context switch.',
    detail: 'The central knob of Round-Robin. Small quantum = more responsiveness, more overhead. Big quantum = less overhead, less responsiveness. In our simulator it is tunable live from the topbar.',
  },
  'Round-Robin (RR)': {
    short: 'Each process gets the CPU in equal turns.',
    detail: 'The simplest and fairest scheduler. Maintains a circular ready queue. When the timer fires, the current process moves to the tail and the head one becomes RUN. Every process gets the same time.',
  },
  Preemption: {
    short: 'The kernel takes the CPU away from the process even if it did not want to yield.',
    detail: 'In preemptive multitasking, the timer forces the switch. The opposite is "cooperative" where the process must yield voluntarily — risky, an infinite loop hangs the system.',
  },
  Turnaround: {
    short: 'Total time from when the process enters until it ends (ZOMBIE).',
    detail: 'TAT = endedAtTick − createdAtTick. Measures the total latency the user perceives for their work to complete. A good scheduler tends to minimize average TAT without sacrificing fairness between processes. The panel shows "—" while the process is still alive.',
  },
  'Waiting time': {
    short: 'Sum of ticks the process spent in the READY queue without running.',
    detail: 'WT = totalReadyTicks. Does not include time in BLOCKED (that is resource wait, not CPU wait). Under Round-Robin, WT grows proportionally to how many processes compete for the CPU. The most common metric to judge fairness between processes.',
  },
  'Response time': {
    short: 'Ticks between process creation and the first time it gets the CPU.',
    detail: 'RT = firstRunAtTick − createdAtTick. Important in interactive systems: if a command takes 200 ms to start executing, the user perceives it as slow even if TAT is good. Under Round-Robin, RT is bounded by (N−1)·quantum where N is the number of READY processes.',
  },
  Throughput: {
    short: 'Processes finished per time unit.',
    detail: 'Throughput = ZOMBIE_processes / elapsed_time. Here we express it as "procs per 1000 ticks" to keep the scale readable. Grows when the scheduler lets short processes finish quickly; drops when there is too much context-switch overhead (quantum too small) or many blockings.',
  },

  // ─── Concurrency ────────────────────────────────────────────────────────
  'Critical section': {
    short: 'Critical section: a stretch of code that must NOT execute on two threads at once.',
    detail: 'If two threads modify the same variable without protecting the access, results get corrupted (race condition). The critical section is the block that touches the shared resource; it must be guarded by a mutual-exclusion mechanism (mutex, semaphore, atomic instruction). Keep it as short as possible: the longer it is, the more contention.',
  },
  'Race condition': {
    short: 'Race condition: the result depends on the execution order of threads.',
    detail: 'Appears when two threads access the same resource without synchronization and at least one writes. The race_counter demo shows it: two workers incrementing a shared counter lose increments because the CPU\'s load-modify-store sequence is not atomic. Prevented by wrapping the access in a critical section.',
  },
  Mutex: {
    short: 'Mutual exclusion lock: guarantees that only one thread enters the critical section.',
    detail: 'Has two states: free or held by a single holder. The mutex_lock syscall blocks the thread if the mutex is already held (sends it to BLOCKED on the mutex queue); mutex_unlock releases it and wakes the first waiter. In this simulator each mutex appears at /concurrencia with its holder and its wait queue.',
  },
  Semaphore: {
    short: 'Semaphore: counter with atomic wait (P) and signal (V) operations.',
    detail: 'Generalises the mutex: the counter allows up to N concurrent threads in the protected section. wait decrements the counter (blocks the thread if 0); signal increments (wakes a waiter if any). Useful for limiting access to a resource pool or coordinating producer-consumer.',
  },
  'Atomic operation': {
    short: 'Atomic operation: the hardware guarantees it runs without being interrupted.',
    detail: 'In x86, instructions such as LOCK XADD, LOCK CMPXCHG or XCHG lock the bus during the load-modify-store. They are the primitive that mutexes, spinlocks and lock-free counters are built on. Without atomicity, i++ decomposes into MOV/INC/MOV — interruptible and therefore unsafe between threads.',
  },
  Deadlock: {
    short: 'Mutual blocking: two or more threads wait for each other in a cycle and none progresses.',
    detail: 'Coffman\'s four conditions must all hold simultaneously: mutual exclusion, hold-and-wait, no preemption and circular wait. The dining_philosophers demo reproduces the classic case — each philosopher takes one fork and waits for the other, forming a cycle. Prevented by breaking any of the four (for example, taking forks in a global order).',
  },
  'Lock contention': {
    short: 'Contention: many threads compete for the same lock and serialize.',
    detail: 'Each time a thread finds the mutex held, it blocks and joins the queue — pure overhead. A short critical section reduces contention; a fine per-resource lock distributes it. At /concurrencia, the waiter queue of each mutex is the visual thermometer of how much contention there is.',
  },
  'Producer-consumer': {
    short: 'Producer-consumer pattern: one thread produces data, another processes it, sharing a buffer.',
    detail: 'The shared buffer needs protection (mutex) and coordination (two semaphores: empty and full). The producer waits on empty before writing; the consumer waits on full before reading. It is the basis of pipelines, message queues and many async I/O APIs.',
  },
  'Dining philosophers': {
    short: 'Dining philosophers: canonical example of deadlock with N threads sharing N resources.',
    detail: 'Five philosophers share a table with five forks; each needs two forks to eat. If they all pick up the left one at once, they wait forever for the right one (deadlock). Classic solutions: globally order the forks, or limit to N-1 philosophers eating simultaneously. This simulator has two separate demos: `philos_deadlock` (the philosophers wedge in a cycle and the system stops progressing) and `philos_safe` (one of them inverts the take order — lower-id fork first, then higher — and that asymmetry breaks the circular wait).',
  },
  safe_counter: {
    short: 'Demo of increment protected by a mutex — the counter ends exact.',
    detail: 'Two workers run a loop in parallel that takes a mutex, increments a shared counter and releases. Compared with race_counter, here the final result is deterministic (always 2 × iterations) because the critical section is protected.',
  },
  race_counter: {
    short: 'Demo WITHOUT protection — the counter loses increments due to a race condition.',
    detail: 'Same loop as safe_counter but without the mutex. Since load-modify-store is not atomic, two workers can read the same value before the other writes it, losing the increment. The final total varies between runs; the delta to the theoretical result measures how many increments were lost.',
  },

  // ─── Memory ─────────────────────────────────────────────────────────────
  'MMU — Memory Management Unit': {
    short: 'CPU hardware that checks permissions and translates addresses.',
    detail: 'On every memory access, the MMU consults tables (page tables in real x86, regions in our simulator) and decides: is the current mode allowed at this address? If not → page fault. In real x86 it also translates virtual addresses to physical; we simplify to flat memory.',
  },
  'GDT — Global Descriptor Table': {
    short: 'Segment descriptor table.',
    detail: 'In x86 each segment has a descriptor (base, limit, permissions). Selectors (CS/DS/SS) are indices into this table. In our simulator the GDT is nominal — selectors are used only as visual hints, no real translation happens.',
  },
  'TSS — Task State Segment': {
    short: 'Per-core structure where x86 stores the kernel SS:ESP and other task-switch data.',
    detail: 'In real x86, when an interrupt changes the privilege from PL3 to PL0, the HW consults the core\'s TSS to know which address to reload SS:ESP — that pair (SS0, ESP0) is the top of the kernel stack. The TSS lives in memory, its address is loaded into TR (Task Register) and its descriptor sits in the GDT. In this simulator the TSS does not exist as its own structure: its two critical fields are simplified to `KernelStackTop` (the role of ESP0) and direct handling inside the CPU. Each core carries its own `KernelStackTop` so concurrent traps don\'t step on each other\'s frames. On kernel entry from user, the user ESP is saved in `SavedUserESP` — the role that in real x86 the TSS fulfills on the switch — and IRET restores it on returning to PL3.',
  },
  'Page fault': {
    short: 'The memory access is invalid and the MMU rejects it.',
    detail: 'Typical causes: the PL3 process tries to touch kernel memory, writes to a read-only page (like user_code), or accesses an address outside its allocated region. In real kernels a page fault is NOT always fatal — it is also used for on-demand resolution: demand paging, copy-on-write after a fork, lazy heap mapping. The kernel decides case by case: resolve (mapping the page) or kill the process. In this simulator every page fault is fatal: the process becomes ZOMBIE with BlockedOn="page_fault". The HW dispatch goes through vector 0x0E (see that entry in Interrupts).',
  },

  // ─── Syscalls ───────────────────────────────────────────────────────────
  Syscall: {
    short: 'User call into the kernel to request a service.',
    detail: 'The user cannot touch HW directly (PL3). To read a file, write to screen, fork processes, etc., they DO `int 0x80` (or `syscall` on x86-64). The kernel answers. The interface is stable across kernel versions.',
  },
  'sys_write (4)': {
    short: 'write(fd, buf, count) — write bytes to an fd.',
    detail: 'EAX=4, EBX=fd, ECX=buf, EDX=count. fd=1 is stdout. Returns in EAX the number of bytes written or -1 on error.',
  },
  'sys_read (3)': {
    short: 'read(fd, buf, count) — read bytes from an fd.',
    detail: 'EAX=3, EBX=fd, ECX=buf, EDX=count. fd=0 is stdin. If no data is available, BLOCKS the process until it arrives (via the device IRQ).',
  },
  'sys_exit (1)': {
    short: 'exit(status) — terminate the process.',
    detail: 'EAX=1, EBX=status. The process becomes ZOMBIE; if there are other READY processes, the scheduler gives them the CPU. If it was the last one, the CPU halts.',
  },

  // ─── Instructions ───────────────────────────────────────────────────────
  HLT: {
    short: 'Halt: stops the CPU until the next IRQ.',
    detail: "Privileged instruction (PL0 only). In the kernel's idle loop: when there are no READY processes, we HLT with IF=1 — the CPU sleeps until the timer or another IRQ wakes it.",
  },
  NOP: {
    short: 'No operation. Burns one cycle.',
    detail: 'Useful for padding (alignment), debugging, or as a placeholder. In our minimalist handlers we use `nop iret` so the dispatch is visible.',
  },
  'JMP / JE / JNE / JG / JL': {
    short: 'Jumps. Conditionals read the flags.',
    detail: 'JMP is unconditional. JE/JZ jumps if ZF=1. JNE/JNZ if ZF=0. JG/JL for signed comparisons (read SF and OF). Useful after a CMP to make decisions.',
  },
  'CMP a, b': {
    short: 'Compare: does (a - b) and updates flags, without writing the result.',
    detail: 'Equivalent to SUB but discards the result. Only leaves the flags. Typically followed by a conditional jump.',
  },
};
