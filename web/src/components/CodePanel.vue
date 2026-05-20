<script setup lang="ts">
import { useCpuStore } from '../stores/cpu';
import { computed, ref, watch } from 'vue';
import { MousePointerClick } from '@lucide/vue';

const cpu = useCpuStore();

// PID actualmente seleccionado para ver su código. Sigue al currentPID por
// default, pero el user puede pinearlo a otro proceso.
const pinnedPID = ref<number | null>(null);
const viewingPID = computed(() => pinnedPID.value ?? cpu.currentPID);

// Si el currentPID cambia y NO estamos pineados, seguimos al actual.
watch(() => cpu.currentPID, (next) => {
  if (pinnedPID.value === null && next > 0) {
    // nada que hacer — el computed ya lo refleja
  }
});

const source = computed(() => {
  if (!viewingPID.value) return '';
  return cpu.procSources[viewingPID.value] ?? '';
});

const currentProc = computed(() =>
  cpu.procs.find(p => p.pid === viewingPID.value)
);

const sortedProcs = computed(() => [...cpu.procs].sort((a, b) => a.pid - b.pid));

function pickProc(pid: number) {
  if (pinnedPID.value === pid) {
    pinnedPID.value = null; // unpin
  } else {
    pinnedPID.value = pid;
  }
}
</script>

<template>
  <div class="panel code">
    <div class="panel-title">
      <span class="title-text">CÓDIGO C · FUENTE DE LOS PROCESOS</span>
      <span class="title-hint" v-if="cpu.procs.length > 1">
        <MousePointerClick :size="12" :stroke-width="2" />
        click en una pestaña para fijar; en blanco sigue al RUN
      </span>
    </div>

    <div v-if="sortedProcs.length === 0" class="empty">
      Cargue un demo para ver el código C de los procesos.
    </div>

    <template v-else>
      <div class="proc-tabs">
        <button
          v-for="p in sortedProcs"
          :key="p.pid"
          class="proc-tab"
          :class="{
            current: p.pid === cpu.currentPID,
            pinned: pinnedPID === p.pid,
            viewing: p.pid === viewingPID,
          }"
          @click="pickProc(p.pid)"
          :title="`P${p.pid} · ${p.name}\nEstado: ${p.state}\n${pinnedPID === p.pid ? 'PINEADO — click para volver a auto' : 'click para pinear este proceso'}`"
        >
          <span class="pt-pid">P{{ p.pid }}</span>
          <span class="pt-name">{{ p.name }}</span>
          <span v-if="p.pid === cpu.currentPID" class="pt-current">●</span>
          <span v-if="pinnedPID === p.pid" class="pt-pin">📌</span>
        </button>
      </div>

      <div class="code-meta" v-if="currentProc">
        <span class="meta-tag" :title="'Estado actual del proceso seleccionado. Cambiá de proceso con las pestañas de arriba.'">{{ currentProc.state }}</span>
        <span class="meta-info">{{ currentProc.name }} · cargado en
          <span class="addr" :title="'Dirección base del código de este proceso en la memoria física.'">0x{{ (currentProc.code_base || 0).toString(16).toUpperCase() }}</span>
          · stack top
          <span class="addr">0x{{ (currentProc.stack_top || 0).toString(16).toUpperCase() }}</span>
        </span>
      </div>

      <pre v-if="source" class="source">{{ source }}</pre>
      <div v-else class="empty">
        Este proceso no tiene código fuente C asociado.
      </div>

      <div class="explain">
        <details>
          <summary>
            <span class="exp-q">⚠ ¿Por qué el C parece "raro"?</span>
          </summary>
          <p>
            En este simulador <strong>no hay un compilador C real</strong>.
            Cada programa fue <em>compilado a mano</em>: el código C está escrito como guía
            pedagógica y los bytes x86 son hand-encoded en paralelo.
          </p>
          <p>
            Por eso va a ver convenciones inusuales: <code>register int</code> mapea explícitamente a
            registros del CPU (EAX, EBX, ...), las llamadas a <code>write/exit/read</code> son la mini-libc
            que traduce a <code>int 0x80</code>, y los loops están escritos pensando en JMP relativos.
          </p>
          <p>
            El comportamiento <strong>SÍ es real</strong>: el simulador ejecuta los bytes x86 reales
            instrucción por instrucción, despacha IRQs como el HW lo haría, y el kernel responde
            como un OS de verdad.
          </p>
        </details>
      </div>
    </template>
  </div>
</template>

<style scoped>
.code {
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

.proc-tab:hover {
  border-color: var(--text-dim);
  color: var(--text);
}

.proc-tab.current {
  border-color: var(--green);
  color: var(--green);
}

.proc-tab.viewing {
  background: rgba(255, 179, 71, 0.15);
  border-color: var(--amber);
  color: var(--amber);
}

.proc-tab.pinned {
  border-style: double;
  border-width: 3px;
}

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

.meta-info {
  color: var(--text-dim);
}

.addr {
  color: var(--cyan);
  font-family: 'JetBrains Mono', monospace;
}

.source {
  flex: 1;
  overflow-y: auto;
  background: rgba(0, 0, 0, 0.4);
  border: 1px solid var(--border);
  padding: 12px 16px;
  margin: 0;
  font-family: 'JetBrains Mono', monospace;
  font-size: 13px;
  line-height: 1.55;
  color: var(--text);
  white-space: pre-wrap;
  tab-size: 4;
}

.explain {
  margin-top: 8px;
}

.explain summary {
  cursor: pointer;
  color: var(--amber-dim);
  font-size: 12px;
  padding: 4px;
  user-select: none;
}

.explain summary:hover {
  color: var(--amber);
}

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

.explain p:first-of-type {
  margin-top: 4px;
}

.explain code {
  color: var(--cyan);
  background: rgba(106, 212, 255, 0.08);
  padding: 1px 5px;
  font-size: 11px;
}

.explain strong {
  color: var(--text);
}

.explain em {
  color: var(--amber-dim);
}

@media (max-width: 768px) {
  .proc-tab {
    padding: 10px 12px;
    font-size: 13px;
    min-height: 40px;
  }

  .source {
    font-size: 14px;
    padding: 14px;
    line-height: 1.6;
  }

  .code-meta {
    font-size: 13px;
  }

  .title-hint {
    display: none;
  }
}
</style>
