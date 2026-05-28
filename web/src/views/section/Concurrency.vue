<script setup lang="ts">
import { computed, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { useCpuStore } from '../../stores/cpu';
import Term from '../../components/glossary/Term.vue';

const { t } = useI18n();
const cpu = useCpuStore();

onMounted(() => { cpu.connect(); });

function hex(n: number, w = 8) {
  return '0x' + (n >>> 0).toString(16).padStart(w, '0').toUpperCase();
}
function modeName(m: number) { return m === 0 ? 'KERNEL' : 'USER'; }

// When only one core is provisioned the demos that need real parallelism
// won't shine — we still let the student load them (the kernel handles
// single-core gracefully via the timer scheduler) but surface a hint.
const singleCoreHint = computed(() => cpu.numCores < 2);

const cores = computed(() => {
  // Fall back to a synthetic core[0] view when the snapshot has not yet
  // populated cpu.cores (first connection). When the SimBar filter hides a
  // core, drop it from the grid so the layout reflects the same set as the
  // rest of the app.
  if (cpu.cores.length > 0) return cpu.cores.filter(c => cpu.isCoreVisible(c.id));
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
  <section class="conc" data-pizarra-anchor="view-concurrencia">
    <header class="conc__head">
      <h1><Term term="Critical section" mode="title">{{ t('sections.concurrency.title') }}</Term></h1>
      <div class="conc__hint" v-if="singleCoreHint">
        {{ t('sections.concurrency.singleCoreHintPrefix') }}<strong>{{ cpu.numCores }}</strong>{{ t('sections.concurrency.singleCoreHintBody') }}<code>{{ t('sections.concurrency.singleCoreHintEnv') }}</code>{{ t('sections.concurrency.singleCoreHintSuffix') }}
      </div>
      <div class="conc__hint" v-else>
        {{ t('sections.concurrency.multiCoreHintPrefix') }}<strong>{{ cpu.numCores }}</strong>{{ t('sections.concurrency.multiCoreHintSuffix') }}
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
            <Term
              :term="c.halted ? 'HLT' : (c.current_pid === 0 ? 'Ready queue' : 'RUN')"
              mode="label"
              class="core-state"
            >{{ c.halted ? 'HALT' : (c.current_pid === 0 ? 'IDLE' : 'RUN') }}</Term>
            <Term
              :term="c.mode === 0 ? 'KERNEL MODE (PL0)' : 'USER MODE (PL3)'"
              mode="label"
              class="core-mode"
              :class="`mode-${c.mode}`"
            >{{ modeName(c.mode) }}</Term>
            <span class="core-pid">{{ pidShort(c.current_pid) }}</span>
          </header>
          <div class="core-regs">
            <div class="reg-row"><Term term="EAX" mode="label">EAX</Term><code>{{ hex(c.regs.EAX ?? 0) }}</code></div>
            <div class="reg-row"><Term term="EBX" mode="label">EBX</Term><code>{{ hex(c.regs.EBX ?? 0) }}</code></div>
            <div class="reg-row"><Term term="ECX" mode="label">ECX</Term><code>{{ hex(c.regs.ECX ?? 0) }}</code></div>
            <div class="reg-row"><Term term="EDX" mode="label">EDX</Term><code>{{ hex(c.regs.EDX ?? 0) }}</code></div>
            <div class="reg-row separator"><Term term="EIP" mode="label">EIP</Term><code>{{ hex(c.regs.EIP ?? 0) }}</code></div>
            <div class="reg-row"><Term term="ESP" mode="label">ESP</Term><code>{{ hex(c.regs.ESP ?? 0) }}</code></div>
          </div>
        </article>
      </div>

      <aside class="conc__locks">
        <section class="lock-block">
          <h2><Term term="Mutex" mode="title">{{ t('sections.concurrency.mutexesTitle') }}</Term></h2>
          <p v-if="cpu.mutexes.length === 0" class="empty">
            {{ t('sections.concurrency.mutexEmpty') }}
          </p>
          <table v-else>
            <thead>
              <tr>
                <th>{{ t('sections.concurrency.colId') }}</th>
                <th><Term term="Mutex" mode="label">{{ t('sections.concurrency.colHolder') }}</Term></th>
                <th><Term term="Waiting queue" mode="label">{{ t('sections.concurrency.colQueue') }}</Term></th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="m in cpu.mutexes"
                :key="m.id"
                :data-narrator-anchor="`mutex-${m.id}`"
                :class="{ free: m.holder < 0 }"
              >
                <td>#{{ m.id }}</td>
                <td><code v-if="m.holder >= 0" :title="pidName(m.holder)">P{{ m.holder }}</code><span v-else class="muted">{{ t('sections.concurrency.holderFree') }}</span></td>
                <td>
                  <span v-if="m.waiters.length === 0" class="muted">—</span>
                  <span v-else class="waiters">{{ m.waiters.map(w => 'P' + w).join(' → ') }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </section>

        <section class="lock-block">
          <h2><Term term="Semaphore" mode="title">{{ t('sections.concurrency.semsTitle') }}</Term></h2>
          <p v-if="cpu.semaphores.length === 0" class="empty">
            {{ t('sections.concurrency.semsEmpty') }}
          </p>
          <table v-else>
            <thead>
              <tr>
                <th>{{ t('sections.concurrency.colId') }}</th>
                <th><Term term="Semaphore" mode="label">{{ t('sections.concurrency.colCount') }}</Term></th>
                <th><Term term="Waiting queue" mode="label">{{ t('sections.concurrency.colQueue') }}</Term></th>
              </tr>
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
          <h2><Term term="PCB" mode="title">{{ t('sections.concurrency.procStateTitle') }}</Term></h2>
          <table>
            <thead>
              <tr>
                <th><Term term="PID" mode="label">PID</Term></th>
                <th>{{ t('sections.concurrency.colName') }}</th>
                <th>{{ t('sections.concurrency.colState') }}</th>
                <th><Term term="BLOCKED" mode="label">{{ t('sections.concurrency.colBlockedOn') }}</Term></th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="p in cpu.procs" :key="p.pid">
                <td>P{{ p.pid }}</td>
                <td>{{ p.name }}</td>
                <td>
                  <Term
                    :term="p.state"
                    mode="label"
                    :class="['state-tag', `state-${p.state.toLowerCase()}`]"
                  >{{ p.state }}</Term>
                </td>
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
  font-family: var(--font-display);
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
  background: rgba(var(--accent-rgb), 0.05);
}
.conc__hint code {
  background: rgba(var(--accent-rgb), 0.1);
  padding: 0 4px;
  border-radius: var(--radius-sm);
  color: var(--amber);
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
  font-family: var(--font-display);
}
.lock-block p.empty { font-size: 11px; color: var(--text-muted); margin: 0; }
.lock-block code { background: rgba(var(--accent-rgb), 0.1); padding: 0 4px; border-radius: var(--radius-sm); }

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

@media (max-width: 768px) {
  .conc__body { grid-template-columns: 1fr; }
}
</style>
