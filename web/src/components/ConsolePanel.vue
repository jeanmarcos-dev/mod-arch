<script setup lang="ts">
import { useCpuStore } from '../stores/cpu';
import { onUpdated, ref } from 'vue';

const cpu = useCpuStore();
const scroller = ref<HTMLElement | null>(null);
const keyInput = ref<HTMLInputElement | null>(null);

onUpdated(() => {
  if (scroller.value) scroller.value.scrollTop = scroller.value.scrollHeight;
});

// Colorea cada chunk por PID. Hasta 4 procesos → 4 tonos verdes/cyan distintos.
const PID_COLORS = [
  'var(--cyan)',       // PID 0 (kernel / input echo)
  'var(--green)',
  'var(--cyan)',
  'var(--yellow)',
  'var(--amber)',
];

function colorOf(pid: number) {
  return PID_COLORS[pid] ?? 'var(--green)';
}

function onKeyPress(e: KeyboardEvent) {
  // Cada tecla individual se envía como una IRQ separada.
  if (e.key.length === 1) {
    cpu.pushKey(e.key);
    e.preventDefault();
  } else if (e.key === 'Enter') {
    cpu.pushKey('\n');
    e.preventDefault();
  }
}

function focusInput() {
  keyInput.value?.focus();
}
</script>

<template>
  <div class="panel console" data-pizarra-anchor="console-panel">
    <div class="panel-title">
      CONSOLE · sys_write fd=1
      <span class="kbd-hint" @click="focusInput">⌨ clic para escribir</span>
    </div>
    <pre class="output" ref="scroller">
      <span
        v-for="(c, i) in cpu.consoleChunks"
        :key="i"
        :style="{ color: colorOf(c.pid) }"
      >{{ c.text }}</span>
      <span v-if="cpu.consoleChunks.length === 0" class="empty">(sin output todavía)</span>
    </pre>
    <div class="kbd-wrap">
      <label
        class="kbd-label clickable-label"
        title="Este campo simula el teclado físico de la máquina.&#10;Cada tecla que escriba aquí manda una IRQ 0x21 (interrupción de hardware del teclado) al CPU.&#10;Si hay un proceso BLOCKED esperando con sys_read, lo despierta y le entrega el byte.&#10;&#10;— clic para más info —"
        @click="cpu.openGlossary('IRQ 0x21')"
      >⌨ TECLADO HW
        <span class="kbd-help">(cada tecla → IRQ 0x21)</span>
      </label>
      <input
        ref="keyInput"
        name="kbd_input"
        aria-label="Teclado simulado (cada tecla genera IRQ 0x21)"
        class="key-input"
        type="text"
        inputmode="text"
        autocomplete="off"
        autocapitalize="none"
        autocorrect="off"
        spellcheck="false"
        :disabled="cpu.isViewer()"
        :placeholder="cpu.isViewer() ? 'modo espectador: solo el propietario puede escribir' : 'haga clic aquí y escriba para enviar teclas al simulador...'"
        @keypress="onKeyPress"
      />
    </div>
  </div>
</template>

<style scoped>
.console {
  /* min-height en vez de height fijo: el output tenia ~56px utiles
     (160 - title 30 - kbd-wrap 54 - padding 20). Con 240 quedan ~135. */
  min-height: 240px;
  display: flex;
  flex-direction: column;
}

@media (max-width: 768px) {
  .console {
    /* En mobile la consola ocupa todo el alto disponible (es su propia tab). */
    height: 100%;
    flex: 1;
    min-height: 0;
  }

  .output {
    font-size: 22px;
  }

  .key-input {
    font-size: 20px;
    padding: 12px 14px;
    /* Mejor experiencia con el teclado nativo del celular. */
    -webkit-appearance: none;
    appearance: none;
  }

  .kbd-label {
    font-size: 13px;
  }
}

.panel-title {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.kbd-hint {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  color: var(--text-dim);
  cursor: pointer;
  letter-spacing: 0;
  text-transform: none;
}

.kbd-hint:hover {
  color: var(--green);
}

.output {
  flex: 1;
  min-height: 110px;
  overflow-y: auto;
  background: #050808;
  border: 1px solid var(--border);
  padding: 8px 10px;
  font-family: 'VT323', monospace;
  font-size: 20px;
  line-height: 1.2;
  color: var(--green);
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
}

.empty {
  color: var(--text-muted);
  font-style: italic;
  font-family: 'JetBrains Mono', monospace;
  font-size: 14px;
}

.kbd-wrap {
  margin-top: 6px;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.kbd-label {
  display: flex;
  align-items: baseline;
  gap: 8px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  color: var(--green);
  letter-spacing: 1px;
  text-transform: uppercase;
  cursor: pointer;
}

.kbd-label:hover {
  color: var(--green-glow);
  text-shadow: 0 0 6px rgba(127, 255, 170, 0.4);
}

.kbd-help {
  font-size: 10px;
  color: var(--text-dim);
  letter-spacing: 0;
  text-transform: none;
  font-style: italic;
}

.key-input {
  background: #050808;
  border: 1px solid var(--border);
  color: var(--green);
  padding: 6px 10px;
  font-family: 'VT323', monospace;
  font-size: 18px;
  outline: none;
}

.key-input:focus {
  border-color: var(--green);
  box-shadow: 0 0 6px rgba(127, 255, 170, 0.3);
}

.key-input::placeholder {
  color: var(--text-muted);
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  font-style: italic;
}
</style>
