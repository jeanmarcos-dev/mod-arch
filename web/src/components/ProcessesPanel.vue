<script setup lang="ts">
import { useCpuStore, type CoreInfo, type PCBInfo } from '../stores/cpu';
import { computed } from 'vue';

const cpu = useCpuStore();

function hex(n: number, w = 6) {
  return '0x' + (n >>> 0).toString(16).padStart(w, '0').toUpperCase();
}

function stateClass(state: string) {
  return `state-${state.toLowerCase()}`;
}

// Lista ordenada por PID para que la tabla no salte
const sortedProcs = computed(() => [...cpu.procs].sort((a, b) => a.pid - b.pid));

// In multicore each visible core has its own running process. We surface one
// PCB block per visible core so the user does not have to flip focus to see
// what is running on C1, C2, ... Single-core just collapses to one block.
interface PCBView { core: CoreInfo; pcb: PCBInfo }
const visiblePCBs = computed<PCBView[]>(() => {
  const out: PCBView[] = [];
  for (const c of cpu.cores) {
    if (!cpu.isCoreVisible(c.id)) continue;
    if (c.current_pid <= 0) continue;
    const pcb = sortedProcs.value.find((p) => p.pid === c.current_pid);
    if (pcb) out.push({ core: c, pcb });
  }
  return out.sort((a, b) => a.core.id - b.core.id);
});

// PIDs currently running on any visible core. Used by the table to mark the
// rows that are actually live (not just the focused-core one).
const runningPIDs = computed<Set<number>>(() => {
  const s = new Set<number>();
  for (const c of cpu.cores) {
    if (!cpu.isCoreVisible(c.id)) continue;
    if (c.current_pid > 0) s.add(c.current_pid);
  }
  return s;
});

// Live EIP for a PID: prefer the core where it is currently running so the
// value tracks instruction-by-instruction. Falls back to the PCB snapshot
// (stale, but the correct thing for non-running procs).
function liveEIP(pid: number): number {
  for (const c of cpu.cores) {
    if (!cpu.isCoreVisible(c.id)) continue;
    if (c.current_pid === pid) return c.regs.EIP ?? 0;
  }
  return sortedProcs.value.find((p) => p.pid === pid)?.regs.EIP ?? 0;
}

function pcbRegs(regs: Record<string, number>): Array<{ name: string; val: number }> {
  const order = ['EAX', 'EBX', 'ECX', 'EDX', 'ESP', 'EBP', 'EIP'];
  return order.filter(k => k in regs).map(k => ({ name: k, val: regs[k] }));
}
</script>

<template>
  <div class="panel procs" data-pizarra-anchor="procs-panel">
    <div class="panel-title clickable-title" title="Tabla de procesos del sistema con su Process Control Block.&#10;&#10;— click para más info —" @click="cpu.openGlossary('PCB')">PROCESOS · PCB</div>

    <div v-if="sortedProcs.length === 0" class="empty">
      Cargue un demo para ver los procesos.
    </div>

    <table v-else class="proc-table">
      <thead>
        <tr>
          <th class="clickable-th" title="Process ID. Identificador único asignado por el kernel al crear el proceso.&#10;&#10;— click para más info —" @click="cpu.openGlossary('PID')">PID</th>
          <th class="clickable-th" title="Estado actual: RUN=corriendo en CPU, READY=esperando turno, BLOCKED=esperando I/O, ZOMBIE=terminó pero el PCB queda.&#10;&#10;— click para más info —" @click="cpu.openGlossary('RUN')">STATE</th>
          <th title="Nombre del binario que el proceso está ejecutando.">NAME</th>
          <th class="clickable-th" title="Instruction Pointer. Próxima instrucción a ejecutar.&#10;&#10;— click para más info —" @click="cpu.openGlossary('EIP')">EIP</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="p in sortedProcs"
          :key="p.pid"
          :class="[
            stateClass(p.state),
            { current: runningPIDs.has(p.pid), flash: cpu.flashingPIDs.has(p.pid) },
          ]"
          :title="runningPIDs.has(p.pid) ? 'Proceso actualmente en RUN — sus registros viven en el CPU.' : 'Proceso fuera de RUN — su estado completo está preservado en su PCB.'"
        >
          <td class="pid">P{{ p.pid }}</td>
          <td class="state">{{ p.state }}</td>
          <td class="name">{{ p.name }}</td>
          <td class="eip">{{ hex(liveEIP(p.pid)) }}</td>
        </tr>
      </tbody>
    </table>

    <div class="ready-queue" v-if="sortedProcs.length > 0">
      <div
        class="rq-label clickable-label"
        title="Cola FIFO de procesos READY.&#10;&#10;— click para más info —"
        @click="cpu.openGlossary('Ready queue')"
      >READY QUEUE</div>
      <div class="rq-pills">
        <span v-if="cpu.readyQueue.length === 0" class="rq-empty">— vacía —</span>
        <span v-for="pid in cpu.readyQueue" :key="pid" class="rq-pill">P{{ pid }}</span>
      </div>
    </div>

    <div class="wait-queue" v-if="sortedProcs.length > 0">
      <div
        class="rq-label clickable-label"
        title="Cola de procesos BLOCKED esperando un recurso.&#10;&#10;— click para más info —"
        @click="cpu.openGlossary('Waiting queue')"
      >WAITING QUEUE
        <span v-if="cpu.waitQueue.length > 0" class="wq-hint">— bloqueados en I/O</span>
      </div>
      <div class="rq-pills">
        <span v-if="cpu.waitQueue.length === 0" class="rq-empty">— vacía —</span>
        <span v-for="w in cpu.waitQueue" :key="w.pid" class="wq-pill" :title="`P${w.pid} esperando: ${w.reason}. Va a despertarse cuando llegue el dato/evento que pidió.`">
          P{{ w.pid }}
          <span class="wq-reason">{{ w.reason }}</span>
        </span>
      </div>
    </div>

    <div class="pcb-detail" v-if="visiblePCBs.length > 0">
      <div
        class="pcb-section-title clickable-label"
        title="Process Control Block: estructura del kernel que guarda el estado completo de un proceso.&#10;&#10;— click para más info —"
        @click="cpu.openGlossary('PCB')"
      >{{ visiblePCBs.length === 1 ? 'PCB DEL PROCESO ACTUAL' : 'PCB POR NÚCLEO' }}</div>
      <div
        v-for="v in visiblePCBs"
        :key="v.core.id"
        class="pcb-block"
      >
        <div class="pcb-meta">
          <span v-if="cpu.numCores > 1" class="pcb-core" :title="`Núcleo C${v.core.id}`">C{{ v.core.id }}</span>
          <span class="pid-tag">P{{ v.pcb.pid }}</span>
          <span class="pcb-name">{{ v.pcb.name }}</span>
          <span class="pcb-addr" title="Dirección donde el kernel cargó el binario .text de este proceso. Cada proceso ocupa un slot de 256 KB.">@ {{ hex(v.pcb.code_base) }}</span>
        </div>
        <div class="pcb-regs">
          <span v-for="r in pcbRegs(v.core.regs)" :key="r.name" :title="`${r.name} vivo del núcleo C${v.core.id} mientras P${v.pcb.pid} corre allí. Al ceder el CPU el kernel lo copia al PCB.`">
            <span class="r-name">{{ r.name }}</span>
            <span class="r-val">{{ hex(r.val, 8) }}</span>
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.procs {
  width: 280px;
  flex-shrink: 0;
  height: 100%;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
}

@media (max-width: 768px) {
  .procs {
    width: 100%;
    height: auto;
    flex: 1;
  }

  .proc-table {
    font-size: 14px;
  }

  .proc-table td, .proc-table th {
    padding: 8px 6px;
  }
}

.empty {
  color: var(--text-muted);
  font-style: italic;
  padding: 16px 0;
  text-align: center;
}

.proc-table {
  width: 100%;
  font-size: 13px;
  border-collapse: collapse;
}

.proc-table th {
  text-align: left;
  color: var(--cyan);
  font-weight: 400;
  text-transform: uppercase;
  letter-spacing: 1px;
  font-size: 11px;
  padding: 4px 4px;
  border-bottom: 1px dashed var(--border);
}

.proc-table td {
  padding: 5px 4px;
  border-bottom: 1px dashed rgba(var(--accent-rgb), 0.08);
}

.proc-table tr.current td {
  background: rgba(127, 255, 170, 0.06);
}

.proc-table tr.flash td {
  background: rgba(var(--accent-rgb), 0.15);
  animation: flash-green 800ms ease-out;
}

.pid {
  color: var(--amber);
  font-weight: 700;
}

.state {
  letter-spacing: 1px;
  font-size: 11px;
}

.state-run    { color: var(--green); }
.state-ready  { color: var(--yellow); }
.state-blocked{ color: var(--red-soft); }
.state-zombie { color: var(--text-muted); }

.name { color: var(--text); font-size: 13px; }
.eip { color: var(--cyan); font-family: 'JetBrains Mono', monospace; }

.ready-queue {
  margin-top: 12px;
  padding-top: 8px;
  border-top: 1px dashed var(--border);
}

.rq-label {
  color: var(--cyan);
  font-size: 11px;
  letter-spacing: 1px;
  text-transform: uppercase;
  margin-bottom: 6px;
}

.rq-pills {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

.rq-pill {
  padding: 2px 8px;
  background: rgba(255, 213, 79, 0.08);
  border: 1px solid var(--yellow);
  color: var(--yellow);
  font-size: 13px;
  font-weight: 700;
}

.rq-empty {
  color: var(--text-muted);
  font-style: italic;
  font-size: 13px;
}

.wait-queue {
  margin-top: 10px;
  padding-top: 8px;
  border-top: 1px dashed var(--border);
}

.wq-hint {
  color: var(--text-muted);
  font-style: italic;
  font-size: 10px;
  text-transform: none;
  letter-spacing: 0;
  margin-left: 4px;
}

.wq-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 8px;
  background: rgba(255, 122, 122, 0.06);
  border: 1px solid var(--red-soft);
  color: var(--red-soft);
  font-size: 13px;
  font-weight: 700;
  animation: wait-pulse 1.6s ease-in-out infinite;
}

.wq-reason {
  font-size: 11px;
  font-weight: 400;
  color: var(--text-dim);
  text-transform: lowercase;
  letter-spacing: 0;
}

@keyframes wait-pulse {
  0%, 100% { opacity: 1; box-shadow: none; }
  50%      { opacity: 0.7; box-shadow: 0 0 6px rgba(255, 122, 122, 0.3); }
}

.pcb-detail {
  margin-top: 14px;
  padding-top: 10px;
  border-top: 2px solid var(--border-strong);
  background: rgba(var(--accent-rgb), 0.03);
  margin-left: -10px;
  margin-right: -10px;
  padding-left: 10px;
  padding-right: 10px;
  padding-bottom: 4px;
}

.pcb-section-title {
  font-family: var(--font-display);
  font-size: 11px;
  color: var(--amber);
  letter-spacing: 2px;
  text-transform: uppercase;
  margin-bottom: 8px;
}

/* Each running core gets its own PCB block. Faint divider between them so
   they read as a stack without competing with the parent panel border. */
.pcb-block {
  padding: 6px 0;
}
.pcb-block + .pcb-block {
  border-top: 1px dashed rgba(var(--accent-rgb), 0.18);
  margin-top: 4px;
}

.pcb-core {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  letter-spacing: 1px;
  color: var(--amber);
  background: rgba(var(--accent-rgb), 0.12);
  border: 1px solid var(--amber);
  border-radius: var(--radius-sm);
  padding: 1px 5px;
  min-width: 26px;
  text-align: center;
}

.clickable-title, .clickable-label, .clickable-th {
  cursor: pointer;
  transition: color 0.15s ease;
}

.clickable-title:hover, .clickable-label:hover, .clickable-th:hover {
  color: var(--cyan);
  text-shadow: 0 0 6px rgba(106, 212, 255, 0.4);
}

.pcb-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  flex-wrap: wrap;
}

.pid-tag {
  color: var(--amber);
  font-weight: 700;
  background: rgba(var(--accent-rgb), 0.15);
  padding: 2px 6px;
  font-size: 12px;
  border: 1px solid var(--border-strong);
}

.pcb-name {
  color: var(--text);
  font-size: 12px;
}

.pcb-addr {
  color: var(--cyan);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
}

.pcb-regs {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 4px 12px;
  font-size: 13px;
}

.pcb-regs > span {
  display: flex;
  justify-content: space-between;
  cursor: help;
}

.pcb-regs > span:hover {
  background: rgba(var(--accent-rgb), 0.05);
}

.r-name {
  color: var(--cyan);
}

.r-val {
  color: var(--yellow);
  font-family: 'JetBrains Mono', monospace;
}
</style>
