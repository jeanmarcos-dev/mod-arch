<script setup lang="ts">
import { ref, computed, watch } from 'vue';

const props = defineProps<{ open: boolean; initialSearch?: string }>();
const emit = defineEmits<{ (e: 'close'): void }>();

const search = ref('');

// Cuando se abre con una búsqueda inicial, prefiteamos y autoexpandimos el
// primer match para que se vea inmediatamente.
watch(() => [props.open, props.initialSearch], ([opening, term]) => {
  if (opening) {
    search.value = String(term ?? '');
  }
});

interface Entry {
  term: string;
  category: string;
  short: string;
  detail: string;
}

const entries: Entry[] = [
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

const categories = computed(() => {
  const set = new Set(entries.map(e => e.category));
  return Array.from(set);
});

const filtered = computed(() => {
  if (!search.value.trim()) return entries;
  const q = search.value.toLowerCase();
  return entries.filter(e =>
    e.term.toLowerCase().includes(q) ||
    e.short.toLowerCase().includes(q) ||
    e.detail.toLowerCase().includes(q) ||
    e.category.toLowerCase().includes(q),
  );
});

const grouped = computed(() => {
  const out: Record<string, Entry[]> = {};
  for (const e of filtered.value) {
    if (!out[e.category]) out[e.category] = [];
    out[e.category].push(e);
  }
  return out;
});

const expanded = ref<Set<string>>(new Set());
function toggle(term: string) {
  if (expanded.value.has(term)) expanded.value.delete(term);
  else expanded.value.add(term);
}
</script>

<template>
  <div v-if="open" class="backdrop" @click.self="emit('close')">
    <div class="modal">
      <div class="modal-head">
        <h2>⌕ GLOSARIO</h2>
        <input
          v-model="search"
          name="glossary_search"
          aria-label="Buscar en el glosario"
          class="search"
          placeholder="buscar (IRQ, PL, PCB, scheduler...)"
          autofocus
        />
        <button class="close-btn" @click="emit('close')">✕</button>
      </div>

      <div class="modal-body">
        <div v-for="cat in categories" :key="cat">
          <template v-if="grouped[cat]">
            <h3 class="cat-title">{{ cat }}</h3>
            <div
              v-for="e in grouped[cat]"
              :key="e.term"
              class="entry"
              :class="{ expanded: expanded.has(e.term) }"
              @click="toggle(e.term)"
            >
              <div class="entry-row">
                <span class="entry-term">{{ e.term }}</span>
                <span class="entry-short">{{ e.short }}</span>
                <span class="entry-arrow">{{ expanded.has(e.term) ? '▾' : '▸' }}</span>
              </div>
              <div v-if="expanded.has(e.term)" class="entry-detail">
                {{ e.detail }}
              </div>
            </div>
          </template>
        </div>
        <div v-if="filtered.length === 0" class="no-results">
          Sin resultados para "{{ search }}"
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.75);
  z-index: 5000;
  display: flex;
  align-items: center;
  justify-content: center;
  backdrop-filter: blur(2px);
}

.modal {
  width: min(880px, 90vw);
  height: min(720px, 85vh);
  background: var(--bg-1);
  border: 2px solid var(--amber);
  box-shadow: 0 0 40px rgba(255, 179, 71, 0.25);
  display: flex;
  flex-direction: column;
}

.modal-head {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 14px 18px;
  border-bottom: 1px solid var(--border-strong);
  background: rgba(255, 179, 71, 0.06);
}

.modal-head h2 {
  font-family: 'Major Mono Display', monospace;
  font-size: 18px;
  color: var(--amber);
  letter-spacing: 4px;
  text-transform: uppercase;
  margin: 0;
}

.search {
  flex: 1;
  background: rgba(0, 0, 0, 0.4);
  border: 1px solid var(--border-strong);
  color: var(--text);
  padding: 8px 12px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 14px;
  outline: none;
}

.search:focus {
  border-color: var(--amber);
  box-shadow: 0 0 6px rgba(255, 179, 71, 0.3);
}

.close-btn {
  background: transparent;
  border: 1px solid var(--border-strong);
  color: var(--amber);
  width: 36px;
  height: 36px;
  font-size: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
}

.modal-body {
  flex: 1;
  overflow-y: auto;
  padding: 18px;
}

.cat-title {
  font-family: 'Major Mono Display', monospace;
  font-size: 13px;
  color: var(--cyan);
  letter-spacing: 3px;
  text-transform: uppercase;
  margin-top: 16px;
  margin-bottom: 8px;
  padding-bottom: 4px;
  border-bottom: 1px dashed var(--border-strong);
}

.cat-title:first-of-type {
  margin-top: 0;
}

.entry {
  padding: 8px 10px;
  cursor: pointer;
  border-bottom: 1px dashed rgba(255, 179, 71, 0.08);
  transition: background 0.15s ease;
}

.entry:hover {
  background: rgba(255, 179, 71, 0.05);
}

.entry.expanded {
  background: rgba(255, 179, 71, 0.08);
}

.entry-row {
  display: flex;
  align-items: baseline;
  gap: 12px;
}

.entry-term {
  color: var(--amber);
  font-weight: 700;
  font-size: 13px;
  min-width: 180px;
  flex-shrink: 0;
}

.entry-short {
  color: var(--text);
  font-size: 13px;
  flex: 1;
  font-style: italic;
}

.entry-arrow {
  color: var(--amber-dim);
  font-size: 13px;
}

.entry-detail {
  margin-top: 8px;
  padding: 8px 12px;
  background: rgba(0, 0, 0, 0.3);
  border-left: 2px solid var(--cyan);
  color: var(--text);
  font-size: 13px;
  line-height: 1.55;
  font-style: normal;
}

.no-results {
  color: var(--text-muted);
  font-style: italic;
  text-align: center;
  padding: 32px;
}
</style>
