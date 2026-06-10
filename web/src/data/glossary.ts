// Shared glossary entries used by both GlossaryModal (the quick-look popup
// triggered by clicking any term in the simulator) and the /glosario page.
//
// Keeping a single source of truth means a new entry shows up in both places
// automatically. Order inside a category is meaningful — entries are listed
// in the sequence a student is likely to learn them.

import { GLOSSARY_EN } from './glossary.en';
import { GLOSSARY_PT } from './glossary.pt';

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

// Returns the entry with `short`/`detail` swapped for the active locale. When
// the locale is 'es' (or any unknown locale) the original entry is returned
// as-is so the Spanish data file keeps being the source of truth.
export function localizeEntry(entry: GlossaryEntry, locale: string): GlossaryEntry {
  if (locale === 'en') {
    const en = GLOSSARY_EN[entry.term];
    return en ? { ...entry, short: en.short, detail: en.detail } : entry;
  }
  if (locale === 'pt') {
    const pt = GLOSSARY_PT[entry.term];
    return pt ? { ...entry, short: pt.short, detail: pt.detail } : entry;
  }
  return entry;
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
    term: 'RPL — Requested Privilege Level',
    category: 'Modos',
    short: 'Bits 0-1 de un selector de segmento. Indican el privilegio "solicitado" por ese selector.',
    detail: 'Cada selector de 16 bits (CS, DS, SS, etc.) lleva su RPL codificado en los dos bits menos significativos. En user mode los selectores tienen RPL=3 (ej. CS = 0x1B = 0x18 | 3); en kernel los selectores tienen RPL=0 (ej. CS = 0x08). La CPU usa el RPL al ejecutar IRET para decidir si rebaja el privilegio: si el CS popeado tiene RPL=3, además popea SS y ESP del stack y cambia a user mode (inter-privilege IRET). El RPL no debe confundirse con el CPL (Current Privilege Level), que es el RPL del CS actualmente en uso — el CPL es el "modo real" en que está corriendo la CPU; el RPL es lo que pide cada selector cuando se lo carga.',
    aliases: ['CPL', 'current privilege level', 'requested privilege level'],
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
    term: 'ESP — Stack Pointer',
    category: 'Registros',
    short: 'Apunta al tope actual del stack (la última posición ocupada).',
    detail: 'Cada PUSH lo decrementa en 4 bytes y escribe el valor; cada POP lee y lo incrementa en 4. CALL/RET, INT/IRET también lo modifican automáticamente: CALL pushea la dirección de retorno (RET la popea); INT pushea EFLAGS/CS/EIP (IRET los popea). La pila crece HACIA ABAJO — desde direcciones altas hacia bajas — por lo que ESP disminuye a medida que se acumula trabajo. Importante: el valor concreto de ESP depende del modo. En PL3 apunta al stack del proceso de usuario; en PL0 apunta al stack del kernel (la CPU recarga ESP automáticamente al cambiar de privilegio).',
  },
  {
    term: 'EBP — Base Pointer',
    category: 'Registros',
    short: 'Apunta a la base del stack frame actual. Anclaje fijo durante una función.',
    detail: 'A diferencia de ESP, EBP no se mueve con cada PUSH/POP: queda fijo durante toda la ejecución de la función para que el compilador genere accesos predecibles a parámetros y variables locales. La convención clásica es el prólogo `PUSH EBP; MOV EBP, ESP` al entrar, y `POP EBP; RET` al salir. Dentro de la función, los argumentos viven en `[EBP+8]`, `[EBP+12]`, etc. (después del EBP guardado y la dirección de retorno), y las variables locales en `[EBP-4]`, `[EBP-8]`, etc. Resumen: ESP se mueve constantemente; EBP queda quieto.',
    aliases: ['frame pointer'],
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
    detail: 'Selectores de 16 bits que indexan entradas de la GDT (o de la LDT cuando el bit TI=1; este simulador no usa LDT). Los bits 0-1 son el RPL (Requested Privilege Level). En user mode CS = 0x1B (0x18 | 3), en kernel CS = 0x08. IRET lee el CS que va a popear para decidir si vuelve a user o queda en kernel.',
  },
  {
    term: 'CR2 — Control Register 2',
    category: 'Registros',
    short: 'Dirección lineal que provocó el último page fault.',
    detail: 'Cuando la MMU dispara un page fault (vector 0x0E), el HW carga en CR2 la dirección virtual que el proceso intentó tocar. El kernel lee CR2 dentro del handler para decidir qué hacer: resolver la página si la falla es legítima (demand-paging, copy-on-write), o matar al proceso si el acceso es inválido. En este simulador CR2 aparece en el panel de eventos y en el payload del evento page_fault con el nombre canónico `cr2` (también disponible como `addr`).',
    aliases: ['cr2', 'control register 2'],
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
    detail: 'En user mode IF normalmente está en 1 para que el timer pueda preempt. Cuando entra una IRQ, el HW pushea EFLAGS y clearea IF para evitar re-entrancia. IRET restaura el EFLAGS popeado (re-habilita IF si estaba en 1). HLT con IF=1 espera la próxima IRQ. Nota fina: en x86 real este clear de IF depende del tipo de "gate" en la IDT — los interrupt gates clearean IF, los trap gates no. En este simulador toda entrada se trata como interrupt gate.',
  },

  // ─── Interrupciones ─────────────────────────────────────────────────────
  {
    term: 'Vector',
    category: 'Interrupciones',
    short: 'Índice 0-255 en la IDT. Cubre IRQs de HW, excepciones del CPU y software interrupts.',
    detail: 'Todo lo que entra por la IDT lleva un vector que indexa el handler correspondiente. La taxonomía técnica distingue tres orígenes para ese vector: IRQ (hardware externo), excepción (el propio CPU al ejecutar una instrucción) y software interrupt (la instrucción INT N). Coloquialmente — y en el código fuente de este simulador — se usa "IRQ" como término paraguas para los tres; el glosario en cambio respeta la distinción para que usted aprenda la terminología real de x86. Convención de naming en las entradas siguientes: prefijo "Vector NN" para excepciones del CPU (0x00, 0x0D, 0x0E), prefijo "IRQ NN" para interrupciones de hardware externo (0x20, 0x21) y prefijo "INT NN" para software interrupts (0x80, 0x81). El número del vector es el mismo concepto en los tres casos: el origen del trigger es lo que cambia.',
    aliases: ['vector de interrupción', 'vector de interrupcion'],
  },
  {
    term: 'IRQ — Interrupt Request',
    category: 'Interrupciones',
    short: 'Señal externa al CPU pidiéndole atención.',
    detail: 'En sentido estricto, una IRQ es un evento asincrónico que el HW externo (timer, teclado, disco) le manda al CPU. El CPU la atiende entre instrucciones si IF=1. Cada IRQ tiene un vector (número 0-255) que indexa la IDT. Las excepciones (originadas por el CPU) y los software interrupts (instrucción INT N) comparten el mismo dispatch pero NO son IRQs en el sentido riguroso.',
  },
  {
    term: 'Excepción',
    category: 'Interrupciones',
    short: 'Evento generado por el propio CPU al ejecutar una instrucción problemática.',
    detail: 'No la dispara HW externo: la dispara el CPU mismo cuando una instrucción no puede completarse (divide por cero, acceso de memoria inválido, instrucción privilegiada en user mode, etc.). Se subclasifica en fault (EIP apunta a la instrucción ofensora, reintentable), trap (EIP apunta a la siguiente) y abort (no recuperable). Aunque entra por la IDT como una IRQ, técnicamente NO es una IRQ.',
    aliases: ['exception', 'excepcion', 'fault'],
  },
  {
    term: 'Software interrupt',
    category: 'Interrupciones',
    short: 'Trap disparado por la instrucción INT N desde el código.',
    detail: 'El usuario o el kernel ejecuta `INT N`. El HW lee IDT[N], pushea EFLAGS/CS/EIP, cambia a kernel mode y salta al handler. Funcionalmente idéntico al dispatch de una IRQ HW; la diferencia es el origen (`source = SW` vs `HW`). Usado clásicamente para syscalls (Linux i386 usa INT 0x80). NO es una IRQ en sentido estricto, aunque comparta el mecanismo.',
    aliases: ['trap', 'int n', 'software interruption'],
  },
  {
    term: 'Vector 0x00 — Divide error',
    category: 'Interrupciones',
    short: 'Excepción cuando una división entera intenta dividir por cero (o desborda).',
    detail: 'La emite el propio CPU al ejecutar DIV o IDIV con divisor cero, o cuando el cociente no entra en el registro destino. Es una excepción de tipo fault: EIP apunta a la instrucción ofensora, no a la siguiente. El kernel suele terminar el proceso responsable.',
    aliases: ['IRQ 0x00', 'divide error', '#DE'],
  },
  {
    term: 'IRQ 0x20 — Timer',
    category: 'Interrupciones',
    short: 'Tick periódico del reloj programable (PIT).',
    detail: 'Cada `quantum` ticks de CPU se dispara este IRQ HW. El scheduler la usa para preempt: guarda el PCB del proceso actual, elige el próximo, restaura su PCB. Es lo que hace que el SO sea multitarea preemptivo. A diferencia de los vectores 0x00 / 0x0E / 0x80 / 0x81, esta sí es una IRQ en el sentido riguroso: la dispara HW externo (PIT) de forma asincrónica.',
  },
  {
    term: 'IRQ 0x21 — Keyboard',
    category: 'Interrupciones',
    short: 'Una tecla fue presionada.',
    detail: 'El controlador de teclado mete el scancode/byte en un buffer y dispara esta IRQ HW. El handler del kernel lee el byte y, si hay un proceso BLOCKED esperando teclado, le entrega el byte y lo despierta. Igual que el timer, es una IRQ en sentido estricto (HW externo, asincrónica).',
  },
  {
    term: 'KeyboardBuf',
    category: 'Interrupciones',
    short: 'Cola FIFO interna del kernel donde se acumulan los bytes que aún no se entregaron al proceso lector.',
    detail: `Cuando usted presiona una tecla, el handler de IRQ 0x21 mete el byte en el KeyboardBuf. A partir de ahí pueden ocurrir dos escenarios:

• **Delivered (entregado)** — Si hay un proceso BLOCKED esperando con sys_read(stdin), el kernel le copia el primer byte al buffer del proceso y lo pasa a READY. El narrador del teclado lo muestra como "PID N (prog): BLOCKED → READY".

• **Buffered (encolado)** — Si nadie está leyendo, el byte se queda esperando en el KeyboardBuf hasta que algún proceso haga sys_read. El narrador lo muestra como "KBuf push · sin lector (cola: N)".

El badge del panel de consola lee "KBuf {used}/{cap}" con cap = 16, que es un tope puramente didáctico: representa la dimensión típica del buffer en un teclado real. Cuando "used" supera "cap", el badge cambia a rojo y agrega ⚠ — es una alerta visual, no una pérdida de bytes: el slice subyacente sigue creciendo y entregando todo en orden FIFO. La idea pedagógica es que usted vea cómo presionar teclas sin lector tiene un costo (la cola crece) y aprenda a relacionar sys_read con la liberación del buffer.`,
    aliases: ['KBuf', 'buffer del teclado', 'keyboard buffer'],
  },
  {
    term: 'INT 0x80 — Syscall',
    category: 'Interrupciones',
    short: 'Software interrupt: el user pide servicio al kernel (convención Linux i386).',
    detail: 'No es una IRQ de HW: es una instrucción `int 0x80` que el user ejecuta. El HW cambia a kernel mode (igual que con cualquier INT), el kernel dispatcha por EAX al syscall correspondiente. Linux moderno usa `sysenter`/`syscall` por performance, pero `int 0x80` sigue funcionando.',
    aliases: ['IRQ 0x80', 'int 0x80'],
  },
  {
    term: 'INT 0x81 — Mutex/Semáforo',
    category: 'Interrupciones',
    short: 'Software interrupt del simulador para operaciones de mutex y semáforo.',
    detail: 'No es estándar Linux — es una convención de este simulador. EAX selecciona la operación (mutex_lock=1, mutex_unlock=2, sem_wait=3, sem_signal=4); EBX trae el ID del recurso. El handler vive en el kernel y manipula las listas de mutexes y semáforos que usted ve en /concurrencia.',
    aliases: ['IRQ 0x81', 'int 0x81'],
  },
  {
    term: 'Vector 0x0E — Page Fault',
    category: 'Interrupciones',
    short: 'Vector de la excepción de página, disparada por la MMU.',
    detail: 'Es el slot de la IDT al que la MMU envía cualquier acceso de memoria inválido. Como toda excepción, la dispara el CPU mismo (no HW externo): cuando la unidad de memoria detecta que el acceso no se puede satisfacer en el modo actual, recarga ESP al kernel stack, pushea el frame y salta a IDT[0x0E]. Para qué condiciones lo dispara y qué hace el kernel después, ver la entrada "Page fault" en la sección de Memoria.',
    aliases: ['IRQ 0x0E', '#PF'],
  },
  {
    term: 'Vector 0x0D — General Protection',
    category: 'Interrupciones',
    short: 'Excepción #GP: el user intentó algo que su privilegio no permite.',
    detail: 'La dispara el CPU cuando un proceso en PL3 ejecuta una instrucción privilegiada (HLT, IN/OUT, LGDT/LIDT, MOV CRn, etc.) o usa un selector inválido. El kernel termina el proceso responsable con razón "general_protection". En este simulador, la causa más común que verá es HLT desde user mode — recuerde que HLT solo es válida en PL0.',
    aliases: ['IRQ 0x0D', 'general protection', '#GP', 'GPF'],
  },
  {
    term: 'INT N',
    category: 'Interrupciones',
    short: 'Instrucción que dispara un software interrupt.',
    detail: 'El user ejecuta `int N`. El HW lee IDT[N], pushea EFLAGS/CS/EIP, cambia a kernel mode, salta al handler. Funcionalmente IDÉNTICO al dispatch de una IRQ HW; la diferencia es solo el source (SW vs HW).',
  },
  {
    term: 'IRET',
    category: 'Interrupciones',
    short: 'Return from interrupt. Deshace lo que hizo INT/dispatch.',
    detail: 'Popea del stack, EN ESE ORDEN DE POP: primero EIP (porque está en la cima), luego CS, luego EFLAGS. Si el CS popeado es user (RPL=3), además popea ESP y SS y cambia a user mode (esto se llama "inter-privilege IRET"). Es la única forma "limpia" de volver de kernel a user. La memoria física del frame, leída desde direcciones bajas a altas, queda como [EIP][CS][EFLAGS][ESP_user][SS_user]: el orden inverso al de pop porque el stack crece hacia abajo.',
  },
  {
    term: 'IDT — Interrupt Descriptor Table',
    category: 'Interrupciones',
    short: 'Tabla con 256 entradas: vector → handler.',
    detail: 'Vive en memoria kernel. Cada entrada apunta al handler del vector correspondiente — sin importar si ese vector lo dispara una IRQ HW, una excepción del CPU o un INT N. El HW la consulta automáticamente al dispatch. En x86 real cada entrada es un "gate descriptor" de 8 bytes; en nuestro simulador simplificamos a punteros de 4 bytes.',
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
    detail: 'Tras sys_exit (o una excepción mortal) el PCB queda en ZOMBIE: el proceso ya no consume CPU pero su entrada sobrevive para que el padre lea su exit_code con wait() / waitpid(). Si el padre muere antes o nunca lo recoge, init (PID 1) lo adopta y lo reapa apenas el dispatcher lo despierta. En este simulador init siempre corre en segundo plano; con el toggle del panel PROCESOS se puede mostrar u ocultar.',
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
    short: 'Cola de procesos BLOCKED esperando un recurso.',
    detail: 'En este simulador la presentamos como una sola lista por simplicidad. En kernels reales hay UNA waiting queue POR recurso: una por cada dispositivo de I/O (teclado, disco, cada socket de red), una por cada mutex, una por cada semáforo, una por cada cola de mensajes. Eso permite que cuando ocurre un evento — por ejemplo la IRQ del teclado — el kernel sepa exactamente a qué proceso despertar consultando SOLO la cola del recurso correspondiente, sin tener que escanear la lista completa de procesos del sistema. La separación por recurso también permite políticas distintas por dispositivo: una cola de disco puede atender por orden de llegada, una de socket por prioridad, etc.',
  },
  {
    term: 'TID — Thread ID',
    category: 'Procesos',
    short: 'Identificador único de una tarea, sea proceso o hilo.',
    detail: 'En Linux todo es una task: lo que llamamos "proceso" tiene un TID propio, y los hilos del mismo proceso comparten TGID pero cada uno conserva su TID. En este simulador TID coincide con el PID que el kernel asigna a cada task; el alias semántico ayuda a distinguir "soy un hilo" de "soy un proceso" cuando ambos comparten address space.',
    aliases: ['TID', 'thread id'],
  },
  {
    term: 'TGID — Thread Group ID',
    category: 'Procesos',
    short: 'PID del líder del grupo de hilos. Todos los hilos del mismo "proceso" comparten TGID.',
    detail: 'fork() crea un nuevo TGID (= PID del hijo). clone(CLONE_THREAD) hereda el TGID del padre, así getpid() devuelve el mismo valor para todos los hilos del proceso, mientras gettid() devuelve el TID real. En la UI agrupamos las tarjetas por TGID para que cada "proceso" aparezca como un nodo con sus hilos colgando debajo.',
    aliases: ['TGID', 'thread group id'],
  },
  {
    term: 'clone()',
    category: 'Procesos',
    short: 'Syscall primitivo de Linux para crear tasks con sharing flags.',
    detail: 'fork(), vfork() y pthread_create() son casos particulares de clone(). Los flags relevantes acá: CLONE_VM (compartir address space), CLONE_FILES (compartir tabla de fds), CLONE_THREAD (heredar TGID, ser un hilo hermano), CLONE_VFORK (bloquear al padre hasta que el hijo salga). Combinándolos se obtiene el comportamiento clásico de pthread_create: CLONE_VM | CLONE_FILES | CLONE_THREAD.',
    aliases: ['sys_clone'],
  },
  {
    term: 'pthread_create',
    category: 'Procesos',
    short: 'API POSIX para crear un hilo. En glibc se traduce a clone().',
    detail: 'pthread_create(thread, attr, fn, arg) le pide al kernel una task que comparte memoria y fds con el caller, y arranca en fn(arg). En este simulador es un wrapper sobre sys_clone(120) con los flags CLONE_VM|CLONE_FILES|CLONE_THREAD; pthread_join se modela con waitpid sobre el TID.',
    aliases: ['pthread', 'pthread_join', 'pthread_exit'],
  },
  {
    term: 'Race condition',
    category: 'Procesos',
    short: 'Bug donde dos tasks compiten por un dato sin sincronización y el resultado depende del entrelazado.',
    detail: 'El demo threads_race lo muestra: dos hilos hacen counter++ que en realidad es load-inc-store. Si el timer interrumpe entre el load y el store, el otro hilo puede leer el mismo valor viejo y los dos terminan escribiendo el mismo número — un increment se pierde. Con quantum chico y 200 iteraciones el contador final queda muy por debajo de 400. La fix didáctica es threads_safe: rodear el critical section con mutex_lock/unlock.',
    aliases: ['race', 'condición de carrera'],
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
  {
    term: 'Turnaround',
    category: 'Scheduler',
    short: 'Tiempo total desde que el proceso entra hasta que termina (ZOMBIE).',
    detail: 'TAT = endedAtTick − createdAtTick. Mide la latencia total que percibe el usuario para que su trabajo se complete. Un buen scheduler tiende a minimizar el TAT promedio sin sacrificar la equidad entre procesos. En el panel se ve "—" mientras el proceso siga vivo.',
    aliases: ['TAT', 'turnaround time', 'tiempo de retorno'],
  },
  {
    term: 'Waiting time',
    category: 'Scheduler',
    short: 'Suma de ticks que el proceso pasó en la cola READY sin ejecutar.',
    detail: 'WT = totalReadyTicks. No incluye el tiempo en BLOCKED (eso es espera por recurso, no por CPU). Con Round-Robin, WT crece proporcional a la cantidad de procesos que compiten por el CPU. Es la métrica más usada para juzgar fairness entre procesos.',
    aliases: ['WT', 'tiempo de espera'],
  },
  {
    term: 'Response time',
    category: 'Scheduler',
    short: 'Ticks entre la creación del proceso y la primera vez que recibe CPU.',
    detail: 'RT = firstRunAtTick − createdAtTick. Importante en sistemas interactivos: si un comando tarda 200 ms en empezar a ejecutarse, el usuario lo percibe lento aunque el TAT sea bueno. En Round-Robin el RT está acotado por (N−1)·quantum donde N es la cantidad de procesos READY.',
    aliases: ['RT', 'tiempo de respuesta'],
  },
  {
    term: 'Throughput',
    category: 'Scheduler',
    short: 'Procesos terminados por unidad de tiempo.',
    detail: 'Throughput = procesos_ZOMBIE / tiempo_transcurrido. Acá lo expresamos como "procs por cada 1000 ticks" para que la escala sea legible. Sube cuando el scheduler logra que los procesos cortos terminen rápido; cae si hay mucho overhead de context switches (quantum demasiado chico) o muchos bloqueos.',
    aliases: ['rendimiento'],
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
    detail: 'Cinco filósofos comparten una mesa con cinco tenedores; cada uno necesita dos tenedores para comer. Si todos toman el de la izquierda al mismo tiempo, esperan por siempre el de la derecha (deadlock). Soluciones clásicas: ordenar los tenedores globalmente, o limitar a N-1 filósofos comiendo en simultáneo. En este simulador hay dos demos separados: `philos_deadlock` (los filósofos se traban en círculo y el sistema deja de progresar) y `philos_safe` (uno de ellos invierte el orden de toma — primero el tenedor de menor id, después el de mayor — y esa asimetría rompe la espera circular).',
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
    term: 'TSS — Task State Segment',
    category: 'Memoria',
    short: 'Estructura por núcleo donde x86 guarda SS:ESP del kernel y otros datos del task switch.',
    detail: 'En x86 real, cuando una interrupción cambia el privilegio de PL3 a PL0, el HW consulta la TSS del núcleo para saber a qué dirección recargar SS:ESP — esa pareja (SS0, ESP0) es la cima del kernel stack. La TSS vive en memoria, su dirección se carga en el registro TR (Task Register) y su descriptor está en la GDT. En este simulador la TSS no existe como estructura propia: sus dos campos críticos se simplifican a `KernelStackTop` (rol de ESP0) y al manejo directo dentro de la CPU. Cada núcleo lleva su propio `KernelStackTop` para que traps concurrentes no se pisen los frames. Al entrar al kernel desde user, el ESP de usuario se guarda en `SavedUserESP` — rol que en x86 real cumple precisamente la TSS al hacer el switch — e IRET lo restaura al volver a PL3.',
    aliases: ['task state segment'],
  },
  {
    term: 'Page fault',
    category: 'Memoria',
    short: 'El acceso de memoria es inválido y la MMU lo rechaza.',
    detail: 'Causas típicas: el proceso en PL3 intenta tocar memoria del kernel, escribe en una página de solo lectura (como user_code), o accede a una dirección fuera de su región asignada. En kernels reales el page fault NO es siempre fatal — también se usa para resolución on-demand: paginación demand-paging, copy-on-write tras un fork, mapeo lazy de heap. El kernel decide caso por caso: resolver (mapeando la página) o matar al proceso. En este simulador todo page fault es fatal: el proceso pasa a ZOMBIE con BlockedOn="page_fault". El dispatch del HW va por el vector 0x0E (ver esa entrada en Interrupciones).',
    aliases: ['page fault', 'pagefault'],
  },
  {
    term: 'Error code (page fault)',
    category: 'Memoria',
    short: 'Bitmask de 32 bits que describe la causa del page fault.',
    detail: 'Sigue el layout del Intel SDM Vol 3 §6.15: bit 0 (P) = 1 si fue violación de permisos, 0 si la página no estaba mapeada; bit 1 (W) = 1 si el acceso era de escritura, 0 si era lectura; bit 2 (U) = 1 si la falla vino de PL3 (user), 0 si de PL0 (kernel); bit 4 (I-D) = 1 si fue un fetch de instrucción. Ejemplo: error_code = 7 = 0b111 → bits P, W, U activos → escritura desde user a una página presente pero protegida. En x86 real el HW pushea este código al stack tras el frame; este simulador lo escribe en AddrErrCodeMirror para que el handler lo lea con un `mov` sin manejar layouts asimétricos.',
    aliases: ['error_code', 'pf error code', 'error code'],
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

export function filterEntries(query: string, locale: string = 'es'): GlossaryEntry[] {
  const q = query.trim().toLowerCase();
  if (!q) return GLOSSARY_ENTRIES.map(e => localizeEntry(e, locale));
  return GLOSSARY_ENTRIES
    .map(e => localizeEntry(e, locale))
    .filter(e =>
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
