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

// Refresh automático cuando cambia EIP/ESP del core enfocado, el tick, la vista
// o la dirección custom. La lista de hits per-core se recomputa con el ref de
// `cpu.cores` directamente y no requiere re-fetch.
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

// Multi-core mirror: when more than one core exists we no longer paint the
// hex dump from the focused-core mirror alone — every visible core stamps
// its own EIP/ESP marker so a 2-core demo shows two simultaneous cursors,
// each tinted with its own --core-${id} hue. Single-core keeps the legacy
// behavior so the look does not change for the common case.
const multicore = computed(() => cpu.cores.length > 1);

// Map address → list of cores whose EIP currently lives there. We track a
// list (not just a number) so two cores that happen to share an EIP both
// show up — the renderer still picks the first one for the background to
// keep the byte readable, but the title attribute lists all owners.
const eipByAddr = computed<Map<number, number[]>>(() => {
  const m = new Map<number, number[]>();
  if (!multicore.value) {
    m.set(currentEIP.value, [cpu.selectedCoreId]);
    return m;
  }
  for (const c of cpu.cores) {
    if (!cpu.isCoreVisible(c.id)) continue;
    const eip = (c.regs.EIP ?? 0) >>> 0;
    const prev = m.get(eip);
    if (prev) prev.push(c.id); else m.set(eip, [c.id]);
  }
  return m;
});

// Same idea for ESP, expanded across the 4 bytes that make up the top of
// stack word so the highlight matches the legacy look.
const espByAddr = computed<Map<number, number[]>>(() => {
  const m = new Map<number, number[]>();
  function stampEsp(esp: number, id: number) {
    for (let off = 0; off < 4; off++) {
      const a = (esp + off) >>> 0;
      const prev = m.get(a);
      if (prev) prev.push(id); else m.set(a, [id]);
    }
  }
  if (!multicore.value) {
    stampEsp(currentESP.value, cpu.selectedCoreId);
    return m;
  }
  for (const c of cpu.cores) {
    if (!cpu.isCoreVisible(c.id)) continue;
    stampEsp((c.regs.ESP ?? 0) >>> 0, c.id);
  }
  return m;
});

// Resolve the inline-style override for a byte. EIP wins over ESP because
// the program counter is the more salient marker. Returns a plain record
// shaped like CSSProperties so Vue's :style prop accepts it without a cast.
function hitStyle(addr: number): Record<string, string> | undefined {
  const eip = eipByAddr.value.get(addr);
  if (eip && eip.length > 0) {
    return {
      background: `var(--core-${eip[0] % 4})`,
      color: `var(--core-fg)`,
    };
  }
  const esp = espByAddr.value.get(addr);
  if (esp && esp.length > 0) {
    return {
      boxShadow: `inset 0 -2px 0 var(--core-${esp[0] % 4})`,
    };
  }
  return undefined;
}

function hitTitle(addr: number): string | undefined {
  const eip = eipByAddr.value.get(addr);
  if (eip && eip.length > 0) {
    return `EIP de núcleo${eip.length > 1 ? 's' : ''} ` + eip.map((id) => `C${id}`).join(', ');
  }
  const esp = espByAddr.value.get(addr);
  if (esp && esp.length > 0) {
    return `Tope de pila (ESP) de núcleo${esp.length > 1 ? 's' : ''} ` + esp.map((id) => `C${id}`).join(', ');
  }
  return undefined;
}

// Cores currently visible, sorted so the legend reads left → right C0..C3.
const visibleCores = computed(() => cpu.cores.filter((c) => cpu.isCoreVisible(c.id)).slice().sort((a, b) => a.id - b.id));
</script>

<template>
  <div class="panel mem" data-pizarra-anchor="memory-panel">
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
              'hex-byte--eip': eipByAddr.has(line.addr + j),
              'hex-byte--esp': espByAddr.has(line.addr + j) && !eipByAddr.has(line.addr + j),
            }"
            :style="hitStyle(line.addr + j)"
            :title="hitTitle(line.addr + j)"
            :data-mem-addr="`0x${(line.addr + j).toString(16).toLowerCase()}`"
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
      <!-- Single-core: legacy EIP/ESP swatches. Multi-core: per-core swatches
           so the user can map each highlighted byte back to a core. -->
      <template v-if="!multicore">
        <span class="leg leg-eip">EIP</span>
        <span class="leg leg-esp">ESP</span>
      </template>
      <template v-else>
        <span
          v-for="c in visibleCores"
          :key="`eip-${c.id}`"
          class="leg leg--core"
          :style="{ background: `var(--core-${c.id % 4})`, color: `var(--core-fg)`, borderColor: `var(--core-${c.id % 4})` }"
          :title="`EIP del núcleo C${c.id}`"
        >EIP·C{{ c.id }}</span>
        <span class="leg-sep">·</span>
        <span
          v-for="c in visibleCores"
          :key="`esp-${c.id}`"
          class="leg leg--core-stack"
          :style="{ boxShadow: `inset 0 -2px 0 var(--core-${c.id % 4})`, borderColor: `var(--core-${c.id % 4})`, color: `var(--core-${c.id % 4})` }"
          :title="`Tope de pila (ESP) del núcleo C${c.id}`"
        >ESP·C{{ c.id }}</span>
      </template>
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
  background: rgba(var(--accent-rgb), 0.18);
  color: var(--amber);
  border-color: var(--amber);
}

.addr-input {
  background: var(--surface-overlay);
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
  background: var(--surface-overlay);
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

/* Single-core legacy classes kept for the !multicore branch. Multicore paints
   via the inline :style coming from hitStyle(addr) so each core can carry its
   own --core-${id} tint. */
.hex-byte.eip {
  background: rgba(var(--accent-rgb), 0.4);
  color: #000;
  font-weight: 700;
}

.hex-byte.esp {
  background: rgba(127, 255, 170, 0.25);
  color: var(--green);
}

.hex-byte--eip {
  font-weight: 700;
}

.leg--core,
.leg--core-stack {
  padding: 1px 6px;
  font-weight: 700;
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
.leg-eip { color: #000; background: rgba(var(--accent-rgb), 0.4); border-color: var(--amber); }
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
