<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { MousePointerClick } from '@lucide/vue';
import { useCpuStore, type SourceLine } from '../stores/cpu';

const cpu = useCpuStore();

// PID actually being displayed: follows the focused core's current_pid unless
// the user pins another tab.
const pinnedPID = ref<number | null>(null);
const viewingPID = computed(() => pinnedPID.value ?? cpu.currentPID);

const source = computed<string>(() => {
  if (!viewingPID.value) return '';
  return cpu.procSources[viewingPID.value] ?? '';
});

const sourceMap = computed<SourceLine[]>(() => {
  if (!viewingPID.value) return [];
  return cpu.procSourceMaps[viewingPID.value] ?? [];
});

// Split the C source by '\n' so we can annotate the active line. We keep the
// numbering 1-based to match SourceLine.c_line.
const sourceLines = computed<string[]>(() => source.value.split('\n'));

const currentProc = computed(() =>
  cpu.procs.find(p => p.pid === viewingPID.value),
);

const sortedProcs = computed(() => [...cpu.procs].sort((a, b) => a.pid - b.pid));

// PIDs that are RUN on a visible core right now. Used to mark "live" tabs.
const runningOnVisibleCores = computed(() => new Set(
  cpu.cores.filter(c => cpu.isCoreVisible(c.id) && c.current_pid > 0).map(c => c.current_pid),
));

// The offset of the next instruction to execute for the viewing PID. We
// prefer the EIP of the core currently running this PID; if no core has it,
// we fall back to the PCB's saved registers (state captured at the last
// context switch out).
const currentOffset = computed<number | null>(() => {
  const pid = viewingPID.value;
  if (!pid) return null;
  const base = currentProc.value?.code_base ?? 0;
  const liveCore = cpu.cores.find(c => c.current_pid === pid);
  if (liveCore) {
    return (liveCore.regs.EIP ?? 0) - base;
  }
  const saved = currentProc.value?.regs.EIP ?? 0;
  return saved - base;
});

// Active SourceLine: the entry whose offset matches currentOffset exactly
// (instructions only emit on boundaries). If no exact match, we fall back to
// the largest entry with offset <= currentOffset so users still see SOMETHING
// highlighted while debugging an out-of-bounds EIP.
const activeSourceLine = computed<SourceLine | null>(() => {
  const off = currentOffset.value;
  const map = sourceMap.value;
  if (off === null || map.length === 0) return null;
  const exact = map.find(l => l.offset === off);
  if (exact) return exact;
  let best: SourceLine | null = null;
  for (const l of map) {
    if (l.offset > off) break;
    best = l;
  }
  return best;
});

const activeCLine = computed<number | null>(() => activeSourceLine.value?.c_line ?? null);
const activeOffset = computed<number | null>(() => activeSourceLine.value?.offset ?? null);

function isActiveLine(idx1: number): boolean {
  return activeCLine.value === idx1;
}

function isActiveAsm(offset: number): boolean {
  return activeOffset.value === offset;
}

function pickProc(pid: number) {
  pinnedPID.value = pinnedPID.value === pid ? null : pid;
}

// Auto-scroll: every time the active C line or ASM offset changes, bring the
// matching rows into view inside their own scroll containers. We use
// `block: 'center'` so the highlighted row sits in the middle of the panel
// instead of jumping to the edges on each step.
const cListRef = ref<HTMLElement | null>(null);
const asmListRef = ref<HTMLElement | null>(null);

watch(activeCLine, async () => {
  await nextTick();
  cListRef.value?.querySelector('.line--current')?.scrollIntoView({
    behavior: 'smooth', block: 'center',
  });
});

watch(activeOffset, async () => {
  await nextTick();
  asmListRef.value?.querySelector('.asm-line--current')?.scrollIntoView({
    behavior: 'smooth', block: 'center',
  });
});

function fmtHex(n: number): string {
  return '0x' + (n >>> 0).toString(16).toUpperCase().padStart(4, '0');
}
</script>

<template>
  <div class="panel code-asm">
    <div class="panel-title">
      <span class="title-text">CÓDIGO · C ↔ ASSEMBLER</span>
      <span class="title-hint" v-if="cpu.procs.length > 1">
        <MousePointerClick :size="12" :stroke-width="2" />
        click en una pestaña para fijar; en blanco sigue al RUN
      </span>
    </div>

    <div v-if="sortedProcs.length === 0" class="empty">
      Cargue un demo para ver el código C y su traducción a assembler.
    </div>

    <template v-else>
      <div class="proc-tabs">
        <button
          v-for="p in sortedProcs"
          :key="p.pid"
          class="proc-tab"
          :class="{
            current: runningOnVisibleCores.has(p.pid),
            pinned: pinnedPID === p.pid,
            viewing: p.pid === viewingPID,
          }"
          @click="pickProc(p.pid)"
          :title="`P${p.pid} · ${p.name}\nEstado: ${p.state}\n${pinnedPID === p.pid ? 'PINEADO — click para volver a auto' : 'click para pinear este proceso'}`"
        >
          <span class="pt-pid">P{{ p.pid }}</span>
          <span class="pt-name">{{ p.name }}</span>
          <span v-if="runningOnVisibleCores.has(p.pid)" class="pt-current">●</span>
          <span v-if="pinnedPID === p.pid" class="pt-pin">📌</span>
        </button>
      </div>

      <div class="code-meta" v-if="currentProc">
        <span class="meta-tag" :title="'Estado actual del proceso seleccionado.'">{{ currentProc.state }}</span>
        <span class="meta-info">
          {{ currentProc.name }} · cargado en
          <span class="addr" :title="'Dirección base del código en memoria.'">0x{{ (currentProc.code_base || 0).toString(16).toUpperCase() }}</span>
          · stack top
          <span class="addr">0x{{ (currentProc.stack_top || 0).toString(16).toUpperCase() }}</span>
          <template v-if="activeOffset !== null">
            · ejecutando
            <span class="addr">{{ fmtHex(activeOffset) }}</span>
          </template>
        </span>
      </div>

      <div class="split" :class="{ 'split--no-asm': sourceMap.length === 0 }">
        <section class="pane pane--c">
          <header class="pane-head">
            <span class="pane-title">C</span>
            <span class="pane-sub">{{ sourceLines.length }} líneas</span>
          </header>
          <ol v-if="source" ref="cListRef" class="c-list">
            <li
              v-for="(line, idx) in sourceLines"
              :key="idx"
              :class="['line', { 'line--current': isActiveLine(idx + 1) }]"
            >
              <span class="line-no">{{ idx + 1 }}</span>
              <span class="line-text">{{ line || ' ' }}</span>
            </li>
          </ol>
          <div v-else class="pane-empty">— sin código C asociado a este proceso —</div>
        </section>

        <section v-if="sourceMap.length > 0" class="pane pane--asm">
          <header class="pane-head">
            <span class="pane-title">ASSEMBLER</span>
            <span class="pane-sub">{{ sourceMap.length }} instr.</span>
          </header>
          <ol ref="asmListRef" class="asm-list">
            <li
              v-for="(sl, i) in sourceMap"
              :key="`${sl.offset}-${i}`"
              :class="['asm-line', { 'asm-line--current': isActiveAsm(sl.offset) }]"
              :title="`Línea C ${sl.c_line}`"
            >
              <span class="asm-off">{{ fmtHex(sl.offset) }}</span>
              <span class="asm-mnem">{{ sl.mnemonic }}</span>
              <span class="asm-cline">L{{ sl.c_line }}</span>
            </li>
          </ol>
        </section>

        <section v-else class="pane pane--asm pane--asm-fallback">
          <header class="pane-head">
            <span class="pane-title">ASSEMBLER</span>
            <span class="pane-sub">sin mapping</span>
          </header>
          <div class="pane-empty">
            Este demo no declara un mapping C ↔ ASM. El log de eventos muestra
            cada instrucción a medida que se ejecuta.
          </div>
        </section>
      </div>

      <div class="explain">
        <details>
          <summary>
            <span class="exp-q">⚠ ¿Por qué el C parece "raro"?</span>
          </summary>
          <p>
            En este simulador <strong>no hay un compilador C real</strong>.
            Cada programa fue <em>compilado a mano</em>: el código C está como guía
            pedagógica y los bytes x86 se encodearon en paralelo. El panel ASM
            de la derecha refleja esa traducción manual.
          </p>
          <p>
            La línea resaltada en ambos paneles es la próxima a ejecutarse en el
            CPU del núcleo enfocado. Use STEP para avanzar una instrucción y
            verá cómo una línea de C suele expandirse en varias instrucciones.
          </p>
          <p>
            El comportamiento <strong>SÍ es real</strong>: el simulador ejecuta los
            bytes x86 reales, despacha IRQs como el HW lo haría, y el kernel
            responde como un OS de verdad.
          </p>
        </details>
      </div>
    </template>
  </div>
</template>

<style scoped>
.code-asm {
  flex: 1;
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.panel-title {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 10px;
}

.title-text { color: var(--amber); }

.title-hint {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  color: var(--text-dim);
  font-style: italic;
  text-transform: none;
  letter-spacing: 0;
}

.empty {
  color: var(--text-muted);
  font-style: italic;
  padding: 24px;
  text-align: center;
  font-size: 13px;
}

.proc-tabs {
  display: flex;
  gap: 4px;
  margin-bottom: 8px;
  flex-wrap: wrap;
}

.proc-tab {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  font-size: 11px;
  letter-spacing: 0;
  text-transform: none;
  color: var(--text-dim);
  border-color: var(--border);
  cursor: pointer;
  background: rgba(0, 0, 0, 0.3);
}
.proc-tab:hover { border-color: var(--text-dim); color: var(--text); }
.proc-tab.current { border-color: var(--green); color: var(--green); }
.proc-tab.viewing {
  background: rgba(255, 179, 71, 0.15);
  border-color: var(--amber);
  color: var(--amber);
}
.proc-tab.pinned { border-style: double; border-width: 3px; }
.pt-pid { font-weight: 700; }
.pt-current { color: var(--green); font-size: 9px; }
.pt-pin { font-size: 9px; }

.code-meta {
  display: flex;
  gap: 10px;
  align-items: center;
  margin-bottom: 8px;
  padding-bottom: 6px;
  border-bottom: 1px dashed var(--border);
  font-size: 12px;
  flex-wrap: wrap;
}
.meta-tag {
  padding: 2px 8px;
  background: rgba(106, 212, 255, 0.08);
  color: var(--cyan);
  border: 1px solid var(--cyan);
  font-size: 10px;
  letter-spacing: 1px;
  font-weight: 700;
}
.meta-info { color: var(--text-dim); }
.addr {
  color: var(--cyan);
  font-family: 'JetBrains Mono', monospace;
}

/* Split layout: 2 equal columns when ASM is available, single column on
   fallback. Each pane scrolls independently so the highlighted row can be
   centered without dragging the other side. */
.split {
  flex: 1;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  overflow: hidden;
  min-height: 0;
}
.split--no-asm { grid-template-columns: 1fr; }
.split--no-asm .pane--asm-fallback { display: none; }

.pane {
  display: flex;
  flex-direction: column;
  background: rgba(0, 0, 0, 0.4);
  border: 1px solid var(--border);
  min-height: 0;
}

.pane-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  padding: 4px 10px;
  border-bottom: 1px dashed var(--border);
  background: rgba(255, 179, 71, 0.04);
  font-family: 'Major Mono Display', monospace;
  font-size: 10px;
  letter-spacing: 2px;
  text-transform: uppercase;
}
.pane-title { color: var(--amber); }
.pane-sub { color: var(--text-muted); font-size: 9px; letter-spacing: 1px; }

.pane-empty {
  color: var(--text-muted);
  font-style: italic;
  padding: 18px 14px;
  font-size: 12px;
  text-align: center;
}

.c-list,
.asm-list {
  list-style: none;
  margin: 0;
  padding: 4px 0;
  flex: 1;
  overflow-y: auto;
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  line-height: 1.55;
  counter-reset: none;
}

.line,
.asm-line {
  display: grid;
  align-items: baseline;
  padding: 0 10px;
  white-space: pre;
  border-left: 2px solid transparent;
  transition: background var(--transition-fast), border-color var(--transition-fast);
}

.line {
  grid-template-columns: 28px 1fr;
  gap: 10px;
  color: var(--text);
}
.line-no {
  color: var(--text-muted);
  font-size: 10px;
  text-align: right;
  user-select: none;
}
.line-text { white-space: pre-wrap; }

.line--current {
  background: rgba(255, 179, 71, 0.14);
  border-left-color: var(--amber);
  box-shadow: inset 6px 0 18px -10px rgba(255, 179, 71, 0.55);
}
.line--current .line-no { color: var(--amber); font-weight: 700; }
.line--current .line-text { color: var(--amber); }

.asm-line {
  grid-template-columns: 60px 1fr 32px;
  gap: 8px;
  color: var(--text-dim);
}
.asm-off { color: var(--cyan); font-size: 11px; }
.asm-mnem { color: var(--text); }
.asm-cline {
  color: var(--text-muted);
  font-size: 10px;
  text-align: right;
}

.asm-line--current {
  background: rgba(127, 255, 170, 0.10);
  border-left-color: var(--green);
  box-shadow: inset 6px 0 18px -10px rgba(127, 255, 170, 0.45);
}
.asm-line--current .asm-off { color: var(--green); font-weight: 700; }
.asm-line--current .asm-mnem { color: var(--green); font-weight: 700; }
.asm-line--current .asm-cline { color: var(--green); }

.explain { margin-top: 8px; }
.explain summary {
  cursor: pointer;
  color: var(--amber-dim);
  font-size: 12px;
  padding: 4px;
  user-select: none;
}
.explain summary:hover { color: var(--amber); }
.exp-q { font-style: italic; }
.explain[open] {
  padding: 8px;
  background: rgba(255, 179, 71, 0.04);
  border-left: 2px solid var(--amber);
}
.explain p {
  font-size: 12px;
  line-height: 1.55;
  color: var(--text-dim);
  margin: 6px 0;
}
.explain p:first-of-type { margin-top: 4px; }
.explain strong { color: var(--text); }
.explain em { color: var(--amber-dim); }

@media (max-width: 900px) {
  /* Narrow viewports: stack panes vertically so each one keeps a usable
     reading width. The C panel stays on top because the lesson starts there. */
  .split { grid-template-columns: 1fr; }
  .split .pane--asm { min-height: 200px; }
}
</style>
