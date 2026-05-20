<script setup lang="ts">
import { useCpuStore } from '../stores/cpu';
import { computed, onMounted, ref, watch } from 'vue';

const cpu = useCpuStore();

// Modos de vista: ─ alrededor de EIP (código que va a ejecutar), ─ alrededor de
// ESP (cima de la pila), o ─ una dirección elegida por el user.
const view = ref<'eip' | 'esp' | 'custom'>('eip');
const customAddr = ref('0x100000');
const bytes = ref<number[]>([]);
const baseAddr = ref(0);
const loading = ref(false);
const error = ref('');

const targetAddr = computed(() => {
  if (view.value === 'eip') {
    // 16 bytes antes y 48 después, para ver contexto y lo que viene.
    return Math.max(0, (cpu.regs.EIP >>> 0) - 16);
  }
  if (view.value === 'esp') {
    // Top del stack (donde están los datos pushados).
    return cpu.regs.ESP >>> 0;
  }
  const parsed = parseUserAddr(customAddr.value);
  return parsed >= 0 ? parsed : 0x100000;
});

function parseUserAddr(s: string): number {
  s = s.trim();
  if (s.startsWith('0x') || s.startsWith('0X')) {
    const n = parseInt(s.slice(2), 16);
    return isNaN(n) ? -1 : n;
  }
  const n = parseInt(s, 10);
  return isNaN(n) ? -1 : n;
}

async function load() {
  if (!cpu.connected) return;
  loading.value = true;
  error.value = '';
  try {
    const res = await fetch(`/api/memory?addr=0x${targetAddr.value.toString(16)}&len=128`);
    if (!res.ok) {
      error.value = await res.text();
      return;
    }
    const data = await res.json();
    bytes.value = data.bytes;
    baseAddr.value = data.addr;
  } catch (e: any) {
    error.value = String(e);
  } finally {
    loading.value = false;
  }
}

// Refresh automático cuando cambia EIP/ESP (si estamos en esos modos) o el tick.
watch(() => [cpu.regs.EIP, cpu.regs.ESP, cpu.tick, view.value, customAddr.value], () => {
  load();
});

onMounted(() => {
  load();
});

function hex(n: number, w = 2) {
  return (n >>> 0).toString(16).padStart(w, '0').toUpperCase();
}

// Agrupa los bytes en líneas de 16 para una vista hex clásica.
const lines = computed(() => {
  const out: { addr: number; bytes: number[]; ascii: string }[] = [];
  for (let i = 0; i < bytes.value.length; i += 16) {
    const chunk = bytes.value.slice(i, i + 16);
    const ascii = chunk.map(b => (b >= 0x20 && b < 0x7F) ? String.fromCharCode(b) : '.').join('');
    out.push({ addr: baseAddr.value + i, bytes: chunk, ascii });
  }
  return out;
});

// Identifica la región a la que pertenece una dirección, para colorear.
function regionOf(addr: number): string {
  if (addr < 0x100000) return 'reserved';
  if (addr < 0x200000) return 'kernel';
  if (addr < 0x300000) return 'user_code';
  if (addr < 0x400000) return 'user_stack';
  return 'oob';
}

const currentEIP = computed(() => cpu.regs.EIP >>> 0);
const currentESP = computed(() => cpu.regs.ESP >>> 0);
</script>

<template>
  <div class="panel mem">
    <div class="panel-title">MEMORIA · HEX DUMP</div>

    <div class="controls">
      <div class="view-tabs">
        <button
          @click="view = 'eip'"
          :class="{ active: view === 'eip' }"
          title="Muestra los bytes alrededor del EIP actual: el código que se está ejecutando o por ejecutar."
        >EIP context</button>
        <button
          @click="view = 'esp'"
          :class="{ active: view === 'esp' }"
          title="Muestra la cima del stack del proceso. Lo que se pushea/popea se ve acá."
        >Stack (ESP)</button>
        <button
          @click="view = 'custom'"
          :class="{ active: view === 'custom' }"
          title="Permite saltar a una dirección arbitraria (ej. 0x100000 para ver IDT, 0x101000 para handlers)."
        >Custom</button>
      </div>
      <input
        v-if="view === 'custom'"
        v-model="customAddr"
        name="memory_address"
        aria-label="Dirección de memoria a inspeccionar"
        class="addr-input"
        placeholder="0x100000"
      />
    </div>

    <div v-if="error" class="error">{{ error }}</div>

    <pre v-else class="hex-dump">
      <div v-for="(line, i) in lines" :key="i" class="hex-line" :class="regionOf(line.addr)">
        <span class="hex-addr">{{ hex(line.addr, 6) }}</span>
        <span class="hex-bytes">
          <span
            v-for="(b, j) in line.bytes"
            :key="j"
            class="hex-byte"
            :class="{
              eip: line.addr + j === currentEIP,
              esp: line.addr + j === currentESP || line.addr + j === currentESP + 1 || line.addr + j === currentESP + 2 || line.addr + j === currentESP + 3,
            }"
          >{{ hex(b) }}</span>
        </span>
        <span class="hex-ascii">{{ line.ascii }}</span>
      </div>
    </pre>

    <div class="legend">
      <span class="leg leg-reserved">reserved</span>
      <span class="leg leg-kernel">kernel</span>
      <span class="leg leg-user_code">user_code</span>
      <span class="leg leg-user_stack">user_stack</span>
      <span class="leg-sep">·</span>
      <span class="leg leg-eip">EIP</span>
      <span class="leg leg-esp">ESP</span>
    </div>
  </div>
</template>

<style scoped>
.mem {
  flex: 1;
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.controls {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
  flex-wrap: wrap;
}

.view-tabs {
  display: flex;
  gap: 4px;
}

.view-tabs button {
  font-size: 11px;
  padding: 4px 10px;
  letter-spacing: 0;
  text-transform: none;
}

.view-tabs button.active {
  background: rgba(255, 179, 71, 0.18);
  color: var(--amber);
  border-color: var(--amber);
}

.addr-input {
  background: rgba(0, 0, 0, 0.4);
  border: 1px solid var(--border);
  color: var(--amber);
  padding: 4px 8px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  width: 110px;
}

.hex-dump {
  flex: 1;
  overflow-y: auto;
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  line-height: 1.4;
  background: rgba(0, 0, 0, 0.3);
  border: 1px solid var(--border);
  padding: 8px 10px;
  margin: 0;
}

.hex-line {
  display: flex;
  gap: 12px;
  padding: 1px 0;
}

.hex-addr {
  color: var(--cyan);
  flex-shrink: 0;
}

.hex-bytes {
  flex: 1;
}

.hex-byte {
  display: inline-block;
  padding: 0 2px;
  margin-right: 1px;
  color: var(--text);
}

.hex-byte.eip {
  background: rgba(255, 179, 71, 0.4);
  color: #000;
  font-weight: 700;
}

.hex-byte.esp {
  background: rgba(127, 255, 170, 0.25);
  color: var(--green);
}

.hex-ascii {
  color: var(--text-dim);
  flex-shrink: 0;
  font-family: 'VT323', monospace;
  font-size: 14px;
  letter-spacing: 1px;
}

.hex-line.kernel .hex-addr { color: var(--red-soft); }
.hex-line.user_code .hex-addr { color: var(--green); }
.hex-line.user_stack .hex-addr { color: var(--yellow); }

.legend {
  display: flex;
  gap: 8px;
  margin-top: 6px;
  font-size: 11px;
  flex-wrap: wrap;
}

.leg {
  padding: 1px 6px;
  border: 1px solid;
  font-family: 'JetBrains Mono', monospace;
}

.leg-reserved { color: var(--text-dim); border-color: var(--text-dim); }
.leg-kernel { color: var(--red-soft); border-color: var(--red-soft); }
.leg-user_code { color: var(--green); border-color: var(--green); }
.leg-user_stack { color: var(--yellow); border-color: var(--yellow); }
.leg-eip { color: #000; background: rgba(255, 179, 71, 0.4); border-color: var(--amber); }
.leg-esp { color: var(--green); background: rgba(127, 255, 170, 0.25); border-color: var(--green); }

.leg-sep {
  color: var(--text-muted);
}

.error {
  color: var(--red);
  font-size: 12px;
  padding: 8px;
}

@media (max-width: 768px) {
  .view-tabs button {
    padding: 10px 12px;
    font-size: 13px;
    min-height: 40px;
  }

  .hex-dump {
    /* En mobile el ancho típico no entra 16 bytes — permitimos scroll horizontal
       y aumentamos el font para que sea legible al tap. */
    overflow-x: auto;
    font-size: 13px;
  }

  .hex-line {
    white-space: nowrap;
  }

  .hex-ascii {
    font-size: 15px;
  }

  .addr-input {
    width: 130px;
    font-size: 14px;
    padding: 10px;
  }

  .legend {
    gap: 6px;
    font-size: 10px;
  }
}
</style>
