// Shared glossary entries used by both GlossaryModal (the quick-look popup
// triggered by clicking any term in the simulator) and the /glosario page.
//
// Keeping a single source of truth means a new entry shows up in both places
// automatically. Order inside a category is meaningful — entries are listed
// in the sequence a student is likely to learn them.

export interface GlossaryEntry {
  term: string;
  category: string;
  short: string;
  detail: string;
  // Alternate keys that should resolve to this entry via findEntry().
  // Useful when the canonical term is in English ("Mutex") but the markup
  // surfaces a Spanish label ("semáforo"), or to map ad-hoc tokens like
  // "interbloqueo" → "Deadlock".
  aliases?: string[];
}

export const GLOSSARY_ENTRIES: GlossaryEntry[] = [
  // ─── Modos y privilegios ────────────────────────────────────────────────
  {
    term: 'PL — Privilege Level',
    category: 'Modos',
    short: 'Nivel de privilegio del CPU. PL3 = user, PL0 = kernel.',
    detail: 'x86 define 4 niveles de privilegio (0-3). Convencionalmente PL0 = kernel (todos los permisos) y PL3 = user (acceso restringido). Los niveles intermedios casi nunca se usan. El HW chequea PL en cada acceso a memoria, ejecución de instrucciones privilegiadas, etc. El SW NO puede cambiar el PL directamente — solo el HW lo hace en INT/IRET/excepciones.',
  },
  {
    term: 'USER MODE (PL3)',
    category: 'Modos',
    short: 'Procesos normales. Acceso restringido.',
    detail: 'Programas de aplicación corren en PL3. No pueden tocar memoria kernel, ejecutar HLT, IN/OUT, modificar IDT/GDT, etc. Si lo intentan, el HW dispara una excepción que el kernel maneja.',
  },
  {
    term: 'KERNEL MODE (PL0)',
    category: 'Modos',
    short: 'Privilegio total. Acceso a todo.',
    detail: 'Código del SO corre en PL0. Puede leer/escribir cualquier memoria, ejecutar cualquier instrucción, configurar HW. Se entra desde user via INT (syscall) o automáticamente cuando llega una IRQ/excepción.',
  },
  {
    term: 'Mode switch',
    category: 'Modos',
    short: 'Cambio entre user y kernel. Lo hace el HW, nunca el SW.',
    detail: 'Cuando llega una IRQ o el user hace `int N`, el HW guarda el estado (EFLAGS/CS/EIP) y cambia el modo automáticamente. El kernel solo configura QUÉ handler invocar (vía IDT); el cambio de modo es del HW. En IRET, el HW popea el frame y restaura el modo original.',
  },

  // ─── Registros ──────────────────────────────────────────────────────────
  {
    term: 'EAX, EBX, ECX, EDX',
    category: 'Registros',
    short: 'Registros generales de 32 bits.',
    detail: 'GP = General Purpose. EAX (Accumulator), EBX (Base), ECX (Counter), EDX (Data). Por convención Linux i386 los syscalls usan: EAX = número, EBX = arg1, ECX = arg2, EDX = arg3, EAX = resultado.',
  },
  {
    term: 'ESI, EDI',
    category: 'Registros',
    short: 'Source Index / Destination Index.',
    detail: 'Históricamente usados por instrucciones de string (MOVS, LODS, STOS) que copiaban bytes de [ESI] a [EDI]. Hoy se usan como generales.',
  },
  {
    term: 'ESP, EBP',
    category: 'Registros',
    short: 'Stack Pointer / Base Pointer.',
    detail: 'ESP apunta al tope del stack (donde se pushea/popea). EBP marca la base del stack frame actual de la función. PUSH decrementa ESP por 4 y escribe; POP lee y suma 4. El stack crece HACIA ABAJO.',
  },
  {
    term: 'EIP',
    category: 'Registros',
    short: 'Instruction Pointer. La próxima instrucción a ejecutar.',
    detail: 'Después de fetchear los bytes de una instrucción, EIP apunta a la siguiente. JMP/CALL/RET/INT/IRET lo modifican explícitamente. Nadie escribe EIP directamente con MOV — el ISA no lo permite.',
  },
  {
    term: 'EFLAGS',
    category: 'Registros',
    short: 'Registro de flags (estado y configuración).',
    detail: 'Cada bit es un flag distinto: ZF (Zero), SF (Sign), CF (Carry), OF (Overflow), IF (Interrupt enable), etc. Las instrucciones aritméticas/CMP actualizan los flags; los saltos condicionales (JE/JNE/JG/JL) los consultan.',
  },
  {
    term: 'CS, DS, SS',
    category: 'Registros',
    short: 'Segment selectors (Code / Data / Stack).',
    detail: 'Selectores de 16 bits que indexan entradas de la GDT. Los bits 0-1 son el RPL (Requested Privilege Level). En user mode CS = 0x1B (0x18 | 3), en kernel CS = 0x08. IRET lee el CS que va a popear para decidir si vuelve a user o queda en kernel.',
  },

  // ─── Flags individuales ─────────────────────────────────────────────────
  {
    term: 'ZF — Zero Flag',
    category: 'Flags',
    short: '1 si el último resultado fue 0.',
    detail: 'Lo setean ADD/SUB/CMP/AND/etc. Lo consulta JE/JZ (salta si ZF=1) y JNE/JNZ (salta si ZF=0). CMP es SUB que solo actualiza flags sin escribir resultado.',
  },
  {
    term: 'SF — Sign Flag',
    category: 'Flags',
    short: '1 si el último resultado fue negativo.',
    detail: 'Igual al bit más significativo (bit 31) del resultado. Lo consultan JG, JL, JGE, JLE para comparaciones signadas.',
  },
  {
    term: 'CF — Carry Flag',
    category: 'Flags',
    short: '1 si hubo overflow no signado.',
    detail: 'Por ejemplo, sumar 0xFFFFFFFF + 1 setea CF (rebalsa). Usado por instrucciones que extienden aritmética a 64 bits (ADC, SBB).',
  },
  {
    term: 'OF — Overflow Flag',
    category: 'Flags',
    short: '1 si hubo overflow signado.',
    detail: 'Distinto de CF: detecta overflow EN COMPLEMENTO A DOS. Ej: 0x7FFFFFFF + 1 setea OF (pasa de positivo máximo a negativo mínimo).',
  },
  {
    term: 'IF — Interrupt Flag',
    category: 'Flags',
    short: 'Si IF=1, las IRQs HW pueden interrumpir. Si IF=0, ignoradas.',
    detail: 'En user mode IF normalmente está en 1 para que el timer pueda preempt. Cuando entra una IRQ, el HW pushea EFLAGS y clearea IF (evita re-entrancia). IRET restaura el EFLAGS popeado (re-habilita IF si estaba). HLT con IF=1 espera la próxima IRQ.',
  },

  // ─── Interrupciones ─────────────────────────────────────────────────────
  {
    term: 'IRQ 0x00 — Divide error',
    category: 'Interrupciones',
    short: 'Excepción cuando una división entera intenta dividir por cero (o desborda).',
    detail: 'La emite el propio CPU al ejecutar DIV o IDIV con divisor cero, o cuando el cociente no entra en el registro destino. Es una excepción de tipo fault: EIP apunta a la instrucción ofensora, no a la siguiente. El kernel suele terminar el proceso responsable.',
  },
  {
    term: 'IRQ — Interrupt Request',
    category: 'Interrupciones',
    short: 'Señal externa al CPU pidiéndole atención.',
    detail: 'Una IRQ es un evento asincrónico que el HW externo (timer, teclado, disco) le manda al CPU. El CPU la atiende entre instrucciones si IF=1. Cada IRQ tiene un VECTOR (número 0-255) que indexa la IDT.',
  },
  {
    term: 'IRQ 0x20 — Timer',
    category: 'Interrupciones',
    short: 'Tick periódico del reloj programable (PIT).',
    detail: 'Cada `quantum` ticks de CPU se dispara este IRQ. El scheduler la usa para preempt: guarda el PCB del proceso actual, elige el próximo, restaura su PCB. Es lo que hace que el SO sea multitarea preemptivo.',
  },
  {
    term: 'IRQ 0x21 — Keyboard',
    category: 'Interrupciones',
    short: 'Una tecla fue presionada.',
    detail: 'El controlador de teclado mete el scancode/byte en un buffer y dispara IRQ 0x21. El handler del kernel lee el byte y, si hay un proceso BLOCKED esperando teclado, le entrega el byte y lo despierta.',
  },
  {
    term: 'IRQ 0x80 — Syscall',
    category: 'Interrupciones',
    short: 'Convención Linux: el user pide servicio al kernel.',
    detail: 'No es un IRQ HW: es una instrucción `int 0x80` que el user ejecuta. El HW cambia a kernel mode (igual que con cualquier INT), el kernel dispatcha por EAX al syscall correspondiente. Linux moderno usa `sysenter`/`syscall` por performance, pero `int 0x80` sigue funcionando.',
  },
  {
    term: 'IRQ 0x81 — Mutex/Semáforo',
    category: 'Interrupciones',
    short: 'Syscall del simulador para operaciones de mutex y semáforo.',
    detail: 'No es estándar Linux — es una convención de este simulador. EAX selecciona la operación (mutex_lock=1, mutex_unlock=2, sem_wait=3, sem_signal=4); EBX trae el ID del recurso. El handler vive en el kernel y manipula las listas de mutexes y semáforos que usted ve en /concurrencia.',
    aliases: ['int 0x81'],
  },
  {
    term: 'IRQ 0x0E — Page Fault',
    category: 'Interrupciones',
    short: 'Acceso de memoria inválido detectado por la MMU.',
    detail: 'Cuando user intenta leer/escribir memoria a la que no tiene permiso (ej. memoria kernel, o página no mapeada), la MMU dispara esta excepción. El kernel decide: resolver la falla (paging, COW), o matar el proceso.',
  },
  {
    term: 'INT N',
    category: 'Interrupciones',
    short: 'Instrucción que dispara una interrupción SW.',
    detail: 'El user ejecuta `int N`. El HW lee IDT[N], pushea EFLAGS/CS/EIP, cambia a kernel mode, salta al handler. Funcionalmente IDÉNTICO al dispatch de una IRQ HW; la diferencia es solo el source.',
  },
  {
    term: 'IRET',
    category: 'Interrupciones',
    short: 'Return from interrupt. Deshace lo que hizo INT/dispatch.',
    detail: 'Popea EIP, CS, EFLAGS del stack (en ese orden). Si el CS popeado es user (RPL=3), también popea el ESP/SS originales y cambia a user mode. Es la única forma "limpia" de volver de kernel a user.',
  },
  {
    term: 'IDT — Interrupt Descriptor Table',
    category: 'Interrupciones',
    short: 'Tabla con 256 entradas: vector → handler.',
    detail: 'Vive en memoria kernel. Cada entrada apunta al handler del vector correspondiente. El HW la consulta automáticamente al dispatch. En x86 real cada entrada es un "gate descriptor" de 8 bytes; en nuestro simulador simplificamos a punteros de 4 bytes.',
  },

  // ─── Procesos y scheduler ───────────────────────────────────────────────
  {
    term: 'PCB — Process Control Block',
    category: 'Procesos',
    short: 'Estructura del kernel que guarda el estado de un proceso.',
    detail: 'Por cada proceso, el kernel mantiene un PCB con: PID, nombre, estado, snapshot de TODOS los registros, dirección de su código, dirección de su stack, lista de archivos abiertos, prioridad, etc. En Linux esto se llama `task_struct`.',
  },
  {
    term: 'PID — Process ID',
    category: 'Procesos',
    short: 'Identificador único del proceso.',
    detail: 'El kernel asigna un PID monotónicamente al crear cada proceso. PID 1 es típicamente init. En nuestro simulador, PID 1 = primer proceso del demo, PID 2 = segundo, etc.',
  },
  {
    term: 'RUN',
    category: 'Procesos',
    short: 'Proceso actualmente ejecutándose en el CPU.',
    detail: 'Solo UN proceso a la vez puede estar RUN (en single-core). Sus registros están vivos en el CPU. Cuando el timer dispare, pasará a READY.',
  },
  {
    term: 'READY',
    category: 'Procesos',
    short: 'Listo para correr, esperando turno en la ready queue.',
    detail: 'El scheduler lo va a elegir en la próxima preempción. Sus registros viven en el PCB.',
  },
  {
    term: 'BLOCKED',
    category: 'Procesos',
    short: 'Esperando un evento (I/O, sleep, mutex).',
    detail: 'NO consume CPU. Va a la waiting queue. Cuando el evento que espera ocurre (ej. una IRQ de teclado), el kernel lo mueve a READY.',
  },
  {
    term: 'ZOMBIE',
    category: 'Procesos',
    short: 'Proceso terminó pero el PCB aún existe.',
    detail: 'Cuando un proceso hace sys_exit (o muere por una excepción), el kernel marca su PCB como ZOMBIE. El PCB se mantiene para que el padre pueda leer el exit code con wait(). En nuestro demo lo mantenemos por simplicidad.',
  },
  {
    term: 'Context switch',
    category: 'Procesos',
    short: 'Cambio del proceso que está en RUN.',
    detail: 'Cuando ocurre: 1) Guardamos el estado del actual en su PCB. 2) Elegimos el próximo. 3) Cargamos su PCB al CPU. 4) Continuamos. En nuestro simulador esto pasa entre el dispatch de la IRQ y el IRET — el frame del kernel stack se "parchea" con los regs del nuevo proceso.',
  },
  {
    term: 'Ready queue',
    category: 'Procesos',
    short: 'Cola FIFO de procesos READY.',
    detail: 'El scheduler RR la usa: saca de la cabeza para correr, mete por la cola al que pierde el CPU. Otros schedulers (CFS, prioridades) usan estructuras más complejas.',
  },
  {
    term: 'Waiting queue',
    category: 'Procesos',
    short: 'Cola de procesos BLOCKED en algún recurso.',
    detail: 'Por simplicidad la tenemos como una sola lista. En kernels reales hay UNA waiting queue POR recurso (teclado, cada socket, cada semáforo, etc.). El kernel sabe qué proceso despertar cuando llega el evento.',
  },

  // ─── Scheduler ──────────────────────────────────────────────────────────
  {
    term: 'Quantum',
    category: 'Scheduler',
    short: 'Cuántos ticks de CPU le toca a un proceso antes del context switch.',
    detail: 'El parámetro central del Round-Robin. Quantum chico = más responsividad, más overhead. Quantum grande = menos overhead, menos responsividad. En nuestro simulador es ajustable en vivo desde el topbar.',
  },
  {
    term: 'Round-Robin (RR)',
    category: 'Scheduler',
    short: 'Cada proceso obtiene el CPU por turnos iguales.',
    detail: 'El scheduler más simple y justo. Mantiene una ready queue circular. Cuando el timer dispara, el proceso actual va al final de la cola y el de la cabeza pasa a RUN. Todos reciben el mismo tiempo.',
  },
  {
    term: 'Preemption',
    category: 'Scheduler',
    short: 'El kernel le saca el CPU al proceso aunque no haya querido cederlo.',
    detail: 'En preemptive multitasking, el timer fuerza el switch. Lo opuesto es "cooperative" donde el proceso debe ceder voluntariamente (yield) — riesgoso, un loop infinito cuelga el sistema.',
  },

  // ─── Concurrencia y sincronización ──────────────────────────────────────
  {
    term: 'Critical section',
    category: 'Concurrencia',
    short: 'Sección crítica: tramo de código que NO debe ejecutarse por dos hilos a la vez.',
    detail: 'Si dos hilos modifican la misma variable sin proteger el acceso, los resultados se corrompen (race condition). La sección crítica es el bloque que toca el recurso compartido; debe estar custodiada por un mecanismo de exclusión mutua (mutex, semáforo, instrucción atómica). Manténgala lo más corta posible: cuanto más larga, más contención.',
    aliases: ['sección crítica', 'seccion critica'],
  },
  {
    term: 'Race condition',
    category: 'Concurrencia',
    short: 'Condición de carrera: el resultado depende del orden de ejecución de los hilos.',
    detail: 'Aparece cuando dos hilos acceden al mismo recurso sin sincronización y al menos uno escribe. El demo race_counter lo ilustra: dos workers incrementando un contador compartido pierden incrementos porque la secuencia load-modify-store del CPU no es atómica. Se previene encerrando el acceso en una sección crítica.',
    aliases: ['condición de carrera', 'condicion de carrera'],
  },
  {
    term: 'Mutex',
    category: 'Sincronización',
    short: 'Mutual exclusion lock: garantiza que solo un hilo entra a la sección crítica.',
    detail: 'Tiene dos estados: libre o tomado por un único holder. El syscall mutex_lock bloquea al hilo si el mutex ya está tomado (lo manda a BLOCKED encolado al mutex); mutex_unlock lo libera y despierta al primer waiter. En este simulador cada mutex aparece en /concurrencia con su holder y su cola de espera.',
    aliases: ['mutex'],
  },
  {
    term: 'Semaphore',
    category: 'Sincronización',
    short: 'Semáforo: contador con operaciones atómicas wait (P) y signal (V).',
    detail: 'Generaliza al mutex: el contador permite hasta N hilos simultáneos en la sección protegida. wait decrementa el contador (bloquea al hilo si está en 0); signal incrementa (despierta a un waiter si lo hay). Útil para limitar acceso a un pool de recursos o coordinar productor-consumidor.',
    aliases: ['semáforo', 'semaforo'],
  },
  {
    term: 'Atomic operation',
    category: 'Sincronización',
    short: 'Operación atómica: el hardware garantiza que se ejecuta sin ser interrumpida.',
    detail: 'En x86, instrucciones como LOCK XADD, LOCK CMPXCHG o XCHG bloquean el bus durante el load-modify-store. Son la primitiva sobre la que se construyen los mutexes, spinlocks y contadores lock-free. Sin atomicidad, el incremento i++ se descompone en MOV/INC/MOV — interrumpible y por ende inseguro entre hilos.',
    aliases: ['operación atómica', 'operacion atomica'],
  },
  {
    term: 'Deadlock',
    category: 'Concurrencia',
    short: 'Bloqueo mutuo: dos o más hilos se esperan circularmente y ninguno avanza.',
    detail: 'Las cuatro condiciones de Coffman deben cumplirse en simultáneo: exclusión mutua, hold-and-wait, no preemption y espera circular. El demo dining_philosophers reproduce el caso clásico — cada filósofo toma un tenedor y espera el otro, formando un ciclo. Se previene rompiendo cualquiera de las cuatro (por ejemplo, tomando los tenedores en orden global).',
    aliases: ['interbloqueo', 'bloqueo mutuo'],
  },
  {
    term: 'Lock contention',
    category: 'Concurrencia',
    short: 'Contención: varios hilos compiten por el mismo lock y se serializan.',
    detail: 'Cada vez que un hilo encuentra el mutex tomado, se bloquea y va a la cola — eso es overhead puro. Una sección crítica corta reduce la contención; un lock por recurso fino la distribuye. En /concurrencia, la cola de waiters de cada mutex es el termómetro visual de cuánta contención hay.',
    aliases: ['contención', 'contencion'],
  },
  {
    term: 'Producer-consumer',
    category: 'Concurrencia',
    short: 'Patrón productor-consumidor: un hilo produce datos, otro los procesa, comparten un buffer.',
    detail: 'El buffer compartido necesita protección (mutex) y coordinación (dos semáforos: empty y full). El productor espera empty antes de escribir; el consumidor espera full antes de leer. Es la base de pipelines, message queues y muchas APIs de I/O asíncrono.',
    aliases: ['productor-consumidor', 'productor consumidor'],
  },
  {
    term: 'Dining philosophers',
    category: 'Concurrencia',
    short: 'Cena de los filósofos: ejemplo canónico de deadlock con N hilos compartiendo N recursos.',
    detail: 'Cinco filósofos comparten una mesa con cinco tenedores; cada uno necesita dos tenedores para comer. Si todos toman el de la izquierda al mismo tiempo, esperan por siempre el de la derecha (deadlock). Soluciones clásicas: ordenar los tenedores globalmente, o limitar a 4 filósofos comiendo en simultáneo. El demo dining_philosophers muestra ambas variantes.',
    aliases: ['filósofos comensales', 'filosofos comensales', 'filósofos', 'filosofos'],
  },
  {
    term: 'safe_counter',
    category: 'Demos',
    short: 'Demo de incremento protegido por mutex — el contador termina exacto.',
    detail: 'Dos workers ejecutan en paralelo un loop que toma un mutex, incrementa un contador compartido y libera. Comparado con race_counter, aquí el resultado final es determinístico (siempre 2 × iteraciones) porque la sección crítica está protegida.',
  },
  {
    term: 'race_counter',
    category: 'Demos',
    short: 'Demo SIN protección — el contador pierde incrementos por race condition.',
    detail: 'Mismo loop que safe_counter pero sin mutex. Como el load-modify-store no es atómico, dos workers pueden leer el mismo valor antes de que el otro lo escriba, perdiendo el incremento. El total final varía entre corridas; el delta hasta el resultado teórico mide cuántos incrementos se perdieron.',
  },

  // ─── Memoria ────────────────────────────────────────────────────────────
  {
    term: 'MMU — Memory Management Unit',
    category: 'Memoria',
    short: 'Hardware del CPU que chequea permisos y traduce direcciones.',
    detail: 'En cada acceso a memoria, la MMU consulta tablas (page tables en x86 real, regiones en nuestro simulador) y decide: ¿está permitido el modo actual a esta dirección? Si no → page fault. En x86 real también traduce direcciones virtuales a físicas; nosotros simplificamos a memoria flat.',
  },
  {
    term: 'GDT — Global Descriptor Table',
    category: 'Memoria',
    short: 'Tabla de descriptores de segmentos.',
    detail: 'En x86 cada segmento tiene un descriptor (base, límite, permisos). Los selectores (CS/DS/SS) son índices a esta tabla. En nuestro simulador la GDT es nominal — solo usamos los selectores como pistas visuales, no hay translation real.',
  },
  {
    term: 'Page fault',
    category: 'Memoria',
    short: 'Excepción cuando acceso a memoria es inválido.',
    detail: 'La MMU detecta acceso prohibido (mal permiso, página no presente, etc.) y dispara IRQ 0x0E. En kernels reales, page fault es CRÍTICO porque también dispara para mapear páginas on-demand (lazy allocation). En nuestro demo es siempre fatal: matamos el proceso.',
  },

  // ─── Syscalls ───────────────────────────────────────────────────────────
  {
    term: 'Syscall',
    category: 'Syscalls',
    short: 'Llamada del user al kernel para pedir servicio.',
    detail: 'El user no puede acceder a HW directamente (PL3). Para leer un archivo, escribir a pantalla, crear procesos, etc., HACE `int 0x80` (o `syscall` en x86-64). El kernel responde. La interfaz es estable a través de versiones del kernel.',
  },
  {
    term: 'sys_write (4)',
    category: 'Syscalls',
    short: 'write(fd, buf, count) — escribir bytes a un fd.',
    detail: 'EAX=4, EBX=fd, ECX=buf, EDX=count. fd=1 es stdout. Devuelve en EAX la cantidad de bytes escritos o -1 en error.',
  },
  {
    term: 'sys_read (3)',
    category: 'Syscalls',
    short: 'read(fd, buf, count) — leer bytes de un fd.',
    detail: 'EAX=3, EBX=fd, ECX=buf, EDX=count. fd=0 es stdin. Si no hay datos disponibles, BLOQUEA al proceso hasta que lleguen (vía IRQ del dispositivo).',
  },
  {
    term: 'sys_exit (1)',
    category: 'Syscalls',
    short: 'exit(status) — termina el proceso.',
    detail: 'EAX=1, EBX=status. El proceso pasa a ZOMBIE; si hay otros READY, el scheduler les da el CPU. Si era el último, el CPU halta.',
  },

  // ─── Instrucciones x86 ──────────────────────────────────────────────────
  {
    term: 'HLT',
    category: 'Instrucciones',
    short: 'Halt: detiene el CPU hasta la próxima IRQ.',
    detail: 'Instrucción privilegiada (solo PL0). En el "idle loop" del kernel: cuando no hay procesos READY, hacemos HLT con IF=1 — el CPU duerme hasta que el timer u otro IRQ lo despierte.',
  },
  {
    term: 'NOP',
    category: 'Instrucciones',
    short: 'No operation. Solo gasta un ciclo.',
    detail: 'Útil para padding (alineación), debug, o como placeholder. En nuestros handlers minimalistas usamos `nop iret` para que la dispatch sea visible.',
  },
  {
    term: 'JMP / JE / JNE / JG / JL',
    category: 'Instrucciones',
    short: 'Saltos. Condicionales consultan flags.',
    detail: 'JMP es incondicional. JE/JZ salta si ZF=1. JNE/JNZ si ZF=0. JG/JL para comparaciones signadas (consultan SF y OF). Útiles tras un CMP para tomar decisiones.',
  },
  {
    term: 'CMP a, b',
    category: 'Instrucciones',
    short: 'Compara: hace (a - b) y actualiza flags, sin escribir resultado.',
    detail: 'Equivalente a SUB pero descarta el resultado. Solo deja los flags. Típicamente seguido por un salto condicional.',
  },
];

// Convenience helpers used by both rendering surfaces.

export function listCategories(): string[] {
  const out: string[] = [];
  const seen = new Set<string>();
  for (const e of GLOSSARY_ENTRIES) {
    if (!seen.has(e.category)) {
      seen.add(e.category);
      out.push(e.category);
    }
  }
  return out;
}

export function filterEntries(query: string): GlossaryEntry[] {
  const q = query.trim().toLowerCase();
  if (!q) return GLOSSARY_ENTRIES;
  return GLOSSARY_ENTRIES.filter(e =>
    e.term.toLowerCase().includes(q) ||
    e.short.toLowerCase().includes(q) ||
    e.detail.toLowerCase().includes(q) ||
    e.category.toLowerCase().includes(q),
  );
}

export function groupByCategory(entries: GlossaryEntry[]): Record<string, GlossaryEntry[]> {
  const out: Record<string, GlossaryEntry[]> = {};
  for (const e of entries) {
    (out[e.category] ??= []).push(e);
  }
  return out;
}

// Index built once at module load. Each lookup is O(1).
// Resolution order (first match wins, so exact entries never get clobbered):
//   1. Exact term (case-insensitive).
//   2. Prefix before " — " (so "PL" resolves to "PL — Privilege Level").
//   3. Comma-separated tokens (so "EBX" resolves to "EAX, EBX, ECX, EDX").
//      Parenthetical suffixes are stripped ("sys_write (4)" → "sys_write").
//   4. Explicit aliases.
const lookupIndex = new Map<string, GlossaryEntry>();

function normalize(s: string): string {
  return s.trim().toLowerCase();
}

function indexKey(key: string, entry: GlossaryEntry): void {
  const k = normalize(key);
  if (!k) return;
  if (!lookupIndex.has(k)) lookupIndex.set(k, entry);
}

function buildIndex(): void {
  for (const e of GLOSSARY_ENTRIES) {
    indexKey(e.term, e);
    const dashPart = e.term.split('—')[0];
    if (dashPart && dashPart !== e.term) indexKey(dashPart, e);
    for (const tok of e.term.split(',')) {
      const cleaned = tok.replace(/\s*\(.*\)\s*$/, '');
      indexKey(cleaned, e);
    }
    for (const a of e.aliases ?? []) indexKey(a, e);
  }
}
buildIndex();

export function findEntry(term: string): GlossaryEntry | null {
  return lookupIndex.get(normalize(term)) ?? null;
}
