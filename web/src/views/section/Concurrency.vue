<script setup lang="ts">
import { computed, onMounted } from 'vue';
import { useCpuStore } from '../../stores/cpu';
import SectionPlaceholder from './SectionPlaceholder.vue';

const cpu = useCpuStore();

onMounted(() => { cpu.connect(); });

function hex(n: number, w = 8) {
  return '0x' + (n >>> 0).toString(16).padStart(w, '0').toUpperCase();
}
function modeName(m: number) { return m === 0 ? 'KERNEL' : 'USER'; }

// Concurrency demos are filtered by the Category field the backend now
// includes in each ProgramInfo. Other categories appear in the standard
// picker but not here.
const concurrencyDemos = computed(() =>
  cpu.programs.filter(p => (p.category ?? '') === 'concurrencia'),
);

// When only one core is provisioned the demos that need real parallelism
// won't shine — we still let the student load them (the kernel handles
// single-core gracefully via the timer scheduler) but surface a hint.
const singleCoreHint = computed(() => cpu.numCores < 2);

const cores = computed(() => {
  // Fall back to a synthetic core[0] view when the snapshot has not yet
  // populated cpu.cores (first connection).
  if (cpu.cores.length > 0) return cpu.cores;
  return [{
    id: 0,
    regs: { ...cpu.regs },
    segments: { ...cpu.segments },
    mode: cpu.mode,
    halted: cpu.halted,
    tick: cpu.tick,
    current_pid: cpu.currentPID,
  }];
});

function loadDemo(name: string) { cpu.load(name); }
function pidName(pid: number) {
  const p = cpu.procs.find(x => x.pid === pid);
  return p ? `P${pid} ${p.name}` : `P${pid}`;
}
function pidShort(pid: number) {
  if (pid === 0) return '—';
  return `P${pid}`;
}
</script>

<template>
  <!-- If the backend never sent any concurrency demos (older deploy?) we
       still show the placeholder. Otherwise render the live view. -->
  <SectionPlaceholder
    v-if="concurrencyDemos.length === 0"
    title="Concurrencia"
    tagline="Cuando el backend exponga demos de concurrencia esta sección se llenará automáticamente."
    status="soon"
    :preview="[
      'Dos columnas CPU0 / CPU1 con timeline paralela',
      'Demos pedagógicos: race_counter sin lock vs safe_counter con mutex',
      'Visualización de la ReadyQueue compartida (lock acquire/release)',
      'Producer-consumer con semáforos animado',
      'Dining philosophers con visual de deadlock',
    ]"
  />

  <section v-else class="conc">
    <header class="conc__head">
      <h1>Concurrencia</h1>
      <div class="conc__hint" v-if="singleCoreHint">
        Hoy este simulador está corriendo con <strong>{{ cpu.numCores }}</strong> core.
        Para activar las animaciones de competencia real entre cores, el operador
        debe relanzar el servidor con <code>MODOS_CPU_CORES=2</code> (o más, hasta 4).
        Los demos siguen ejecutándose; el timer del kernel intercala los procesos
        en un solo core mientras tanto.
      </div>
      <div class="conc__hint" v-else>
        Cores activos: <strong>{{ cpu.numCores }}</strong>. Los workers de cada
        demo se reparten entre los cores; el mutex serializa la sección crítica
        cuando lo piden.
      </div>

      <div class="conc__demos">
        <span class="conc__demos-label">Demos disponibles:</span>
        <button
          v-for="d in concurrencyDemos"
          :key="d.name"
          class="conc__demo-btn"
          :class="{ active: cpu.selectedProgram === d.name }"
          :title="d.description"
          @click="loadDemo(d.name)"
        >
          {{ d.name }}
        </button>
      </div>
    </header>

    <div class="conc__body">
      <div class="conc__cores">
        <article
          v-for="c in cores"
          :key="c.id"
          class="core-card"
          :class="{ halted: c.halted, kernel: c.mode === 0 }"
          :data-narrator-anchor="`cpu-${c.id}`"
        >
          <header>
            <span class="core-id">CORE {{ c.id }}</span>
            <span class="core-state">{{ c.halted ? 'HALT' : (c.current_pid === 0 ? 'IDLE' : 'RUN') }}</span>
            <span class="core-mode" :class="`mode-${c.mode}`">{{ modeName(c.mode) }}</span>
            <span class="core-pid">{{ pidShort(c.current_pid) }}</span>
          </header>
          <div class="core-regs">
            <div class="reg-row"><span>EAX</span><code>{{ hex(c.regs.EAX ?? 0) }}</code></div>
            <div class="reg-row"><span>EBX</span><code>{{ hex(c.regs.EBX ?? 0) }}</code></div>
            <div class="reg-row"><span>ECX</span><code>{{ hex(c.regs.ECX ?? 0) }}</code></div>
            <div class="reg-row"><span>EDX</span><code>{{ hex(c.regs.EDX ?? 0) }}</code></div>
            <div class="reg-row separator"><span>EIP</span><code>{{ hex(c.regs.EIP ?? 0) }}</code></div>
            <div class="reg-row"><span>ESP</span><code>{{ hex(c.regs.ESP ?? 0) }}</code></div>
          </div>
        </article>
      </div>

      <aside class="conc__locks">
        <section class="lock-block">
          <h2>Mutexes</h2>
          <p v-if="cpu.mutexes.length === 0" class="empty">Ningún mutex activo. Carga <code>safe_counter</code> para crear uno.</p>
          <table v-else>
            <thead>
              <tr><th>ID</th><th>Holder</th><th>Cola</th></tr>
            </thead>
            <tbody>
              <tr
                v-for="m in cpu.mutexes"
                :key="m.id"
                :data-narrator-anchor="`mutex-${m.id}`"
                :class="{ free: m.holder < 0 }"
              >
                <td>#{{ m.id }}</td>
                <td><code v-if="m.holder >= 0" :title="pidName(m.holder)">P{{ m.holder }}</code><span v-else class="muted">libre</span></td>
                <td>
                  <span v-if="m.waiters.length === 0" class="muted">—</span>
                  <span v-else class="waiters">{{ m.waiters.map(w => 'P' + w).join(' → ') }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </section>

        <section class="lock-block">
          <h2>Semáforos</h2>
          <p v-if="cpu.semaphores.length === 0" class="empty">Ningún semáforo activo todavía.</p>
          <table v-else>
            <thead>
              <tr><th>ID</th><th>Count</th><th>Cola</th></tr>
            </thead>
            <tbody>
              <tr v-for="s in cpu.semaphores" :key="s.id" :data-narrator-anchor="`sem-${s.id}`">
                <td>#{{ s.id }}</td>
                <td><code>{{ s.count }}</code></td>
                <td>
                  <span v-if="s.waiters.length === 0" class="muted">—</span>
                  <span v-else class="waiters">{{ s.waiters.map(w => 'P' + w).join(' → ') }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </section>

        <section class="lock-block">
          <h2>Procesos · estado</h2>
          <table>
            <thead>
              <tr><th>PID</th><th>Nombre</th><th>Estado</th><th>Bloqueado en</th></tr>
            </thead>
            <tbody>
              <tr v-for="p in cpu.procs" :key="p.pid">
                <td>P{{ p.pid }}</td>
                <td>{{ p.name }}</td>
                <td><span :class="['state-tag', `state-${p.state.toLowerCase()}`]">{{ p.state }}</span></td>
                <td><code v-if="p.blocked_on">{{ p.blocked_on }}</code><span v-else class="muted">—</span></td>
              </tr>
            </tbody>
          </table>
        </section>
      </aside>
    </div>
  </section>
</template>

<style scoped>
.conc {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  padding: var(--space-4);
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.conc__head { display: flex; flex-direction: column; gap: var(--space-3); }
.conc__head h1 {
  font-family: 'Major Mono Display', monospace;
  letter-spacing: 4px;
  font-size: 18px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  margin: 0;
}

.conc__hint {
  font-size: 12px;
  line-height: 1.5;
  color: var(--text-dim);
  border-left: 3px solid var(--amber);
  padding: var(--space-2) var(--space-3);
  background: rgba(255, 179, 71, 0.05);
}
.conc__hint code {
  background: rgba(255, 179, 71, 0.1);
  padding: 0 4px;
  border-radius: var(--radius-sm);
  color: var(--amber);
}

.conc__demos { display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap; }
.conc__demos-label {
  font-size: 10px;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--text-muted);
}
.conc__demo-btn {
  font-family: inherit;
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  font-size: 11px;
  padding: 4px 10px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  text-transform: lowercase;
  letter-spacing: 0.5px;
  transition: background var(--transition-fast), color var(--transition-fast), border-color var(--transition-fast);
}
.conc__demo-btn:hover {
  background: rgba(255, 179, 71, 0.08);
  color: var(--amber);
  border-color: var(--border-strong);
}
.conc__demo-btn.active {
  background: rgba(255, 179, 71, 0.16);
  color: var(--amber);
  border-color: var(--amber);
}

.conc__body {
  display: grid;
  grid-template-columns: 3fr 2fr;
  gap: var(--space-4);
  min-height: 0;
}

.conc__cores {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--space-3);
  align-content: start;
}

.core-card {
  background: var(--surface-1);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: var(--space-3);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  transition: border-color var(--transition-base), box-shadow var(--transition-base);
}
.core-card.halted { opacity: 0.55; }
.core-card.kernel { border-color: var(--amber); box-shadow: var(--shadow-glow-amber); }

.core-card header {
  display: grid;
  grid-template-columns: auto 1fr auto auto;
  align-items: center;
  gap: var(--space-2);
  font-size: 11px;
  letter-spacing: 1px;
  text-transform: uppercase;
}
.core-id { color: var(--amber); font-weight: bold; }
.core-state { color: var(--text-dim); font-size: 9px; }
.core-mode { font-size: 9px; padding: 2px 6px; border-radius: var(--radius-sm); border: 1px solid var(--border); }
.core-mode.mode-0 { color: var(--amber); border-color: var(--amber); }
.core-mode.mode-3 { color: var(--text-dim); }
.core-pid { font-family: 'JetBrains Mono', monospace; color: var(--cyan, #80d6ff); }

.core-regs { display: flex; flex-direction: column; gap: 2px; font-family: 'JetBrains Mono', monospace; font-size: 11px; }
.reg-row { display: flex; justify-content: space-between; padding: 2px 0; }
.reg-row span { color: var(--text-muted); letter-spacing: 1px; }
.reg-row code { color: var(--text); }
.reg-row.separator { border-top: 1px dashed var(--border); padding-top: 6px; margin-top: 4px; }

.conc__locks { display: flex; flex-direction: column; gap: var(--space-3); }

.lock-block {
  background: var(--surface-1);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: var(--space-3);
}
.lock-block h2 {
  font-size: 11px;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--amber);
  margin: 0 0 var(--space-2);
  font-family: 'Major Mono Display', monospace;
}
.lock-block p.empty { font-size: 11px; color: var(--text-muted); margin: 0; }
.lock-block code { background: rgba(255, 179, 71, 0.1); padding: 0 4px; border-radius: var(--radius-sm); }

.lock-block table {
  width: 100%;
  border-collapse: collapse;
  font-size: 11px;
  font-family: 'JetBrains Mono', monospace;
}
.lock-block th {
  text-align: left;
  font-weight: normal;
  color: var(--text-muted);
  font-size: 9px;
  text-transform: uppercase;
  letter-spacing: 1.5px;
  padding: 4px 6px;
  border-bottom: 1px dashed var(--border);
}
.lock-block td { padding: 4px 6px; border-bottom: 1px dashed rgba(255, 255, 255, 0.04); }
.lock-block tr.free td:nth-child(2) { color: var(--text-muted); }
.lock-block .muted { color: var(--text-muted); }
.lock-block .waiters { color: var(--yellow, #ffd54f); }

.state-tag {
  display: inline-block;
  padding: 1px 6px;
  border-radius: var(--radius-sm);
  font-size: 9px;
  letter-spacing: 1px;
  border: 1px solid var(--border);
}
.state-run { color: var(--amber); border-color: var(--amber); }
.state-ready { color: var(--cyan, #80d6ff); }
.state-blocked { color: var(--yellow, #ffd54f); border-color: var(--yellow, #ffd54f); }
.state-zombie { color: var(--text-muted); }

@media (max-width: 900px) {
  .conc__body { grid-template-columns: 1fr; }
}
</style>
