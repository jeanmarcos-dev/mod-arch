<script setup lang="ts">
import { computed, nextTick, onMounted, onUpdated, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { Hash, RotateCcw, Clipboard } from '@lucide/vue';
import { useCpuStore } from '../../stores/cpu';

const cpu = useCpuStore();
const { t } = useI18n();

const scroller = ref<HTMLElement | null>(null);
const showHex = ref(false);
const copyFlash = ref(false);

interface ChunkGroup {
  key: string;
  pid: number;
  coreId?: number;
  kind: 'in' | 'out';
  prog: string;
  text: string;
  bytes: number[];
  count: number;
}

// Adjacent chunks from the same (pid, coreId, kind) collapse into one group so
// the monitor reads like a series of speaker turns instead of a wall of
// fragments. Kind is included so a kernel echo ("in") never merges with a
// kernel sys_write ("out") on the same PID.
const groups = computed<ChunkGroup[]>(() => {
  const out: ChunkGroup[] = [];
  for (let i = 0; i < cpu.consoleChunks.length; i++) {
    const c = cpu.consoleChunks[i];
    const kind: 'in' | 'out' = c.kind ?? (c.pid === 0 ? 'in' : 'out');
    const last = out[out.length - 1];
    if (last && last.pid === c.pid && last.coreId === c.coreId && last.kind === kind) {
      last.text += c.text;
      if (c.bytes) last.bytes.push(...c.bytes);
      last.count++;
      continue;
    }
    out.push({
      key: `${i}-${c.pid}-${c.coreId ?? 'x'}-${kind}`,
      pid: c.pid,
      coreId: c.coreId,
      kind,
      prog: cpu.processNameOf(c.pid) || t('panels.console.pidUnknown'),
      text: c.text,
      bytes: c.bytes ? [...c.bytes] : [],
      count: 1,
    });
  }
  return out;
});

const multicore = computed(() => cpu.cores.length > 1);
const empty = computed(() => groups.value.length === 0);

const PID_COLORS = [
  'var(--cyan)',
  'var(--green)',
  'var(--cyan)',
  'var(--yellow)',
  'var(--amber)',
];
function colorOf(pid: number) {
  return PID_COLORS[pid] ?? 'var(--green)';
}

// xxd-style hexdump: each row shows a 16-byte window aligned to its offset.
// Two halves of 8 bytes separated by an extra space make the byte index easier
// to count by eye, and the ASCII column on the right renders printable bytes
// as-is (DEL and control codes collapse to '.'). The trailing space padding
// keeps the ASCII column from sliding left when the final row is short.
function hexOf(bytes: number[]): string {
  const lines: string[] = [];
  const HALF_PAD = 8 * 3 - 1;
  for (let off = 0; off < bytes.length; off += 16) {
    const slice = bytes.slice(off, off + 16);
    const hexParts = slice.map(b => b.toString(16).padStart(2, '0'));
    const leftHex = hexParts.slice(0, 8).join(' ').padEnd(HALF_PAD, ' ');
    const rightHex = hexParts.slice(8, 16).join(' ').padEnd(HALF_PAD, ' ');
    const ascii = slice
      .map(b => (b >= 0x20 && b < 0x7f) ? String.fromCharCode(b) : '.')
      .join('');
    const offHex = off.toString(16).padStart(4, '0');
    lines.push(`${offHex}  ${leftHex}  ${rightHex}  ${ascii}`);
  }
  return lines.join('\n');
}

function scrollToBottom() {
  if (scroller.value) scroller.value.scrollTop = scroller.value.scrollHeight;
}

onMounted(scrollToBottom);
onUpdated(scrollToBottom);
watch(groups, () => nextTick(scrollToBottom), { deep: true });

function onClear() {
  cpu.consoleChunks.splice(0, cpu.consoleChunks.length);
}

async function onCopy() {
  try {
    const text = groups.value.map(g => g.text).join('');
    await navigator.clipboard.writeText(text);
    copyFlash.value = true;
    window.setTimeout(() => { copyFlash.value = false; }, 1200);
  } catch {
    // Some browsers reject writes from unfocused frames — silent fallback.
  }
}
</script>

<template>
  <section class="monitor" :aria-label="t('panels.console.title')">
    <header class="monitor__header">
      <h2 class="monitor__title">{{ t('panels.console.title') }}</h2>
      <div class="monitor__actions">
        <button
          type="button"
          class="monitor__btn"
          :class="{ 'is-active': showHex }"
          :aria-pressed="showHex"
          :aria-label="showHex ? t('panels.console.actions.hexHide') : t('panels.console.actions.hexToggle')"
          :title="showHex ? t('panels.console.actions.hexHide') : t('panels.console.actions.hexToggle')"
          @click="showHex = !showHex"
        ><Hash :size="14" /></button>
        <button
          type="button"
          class="monitor__btn"
          :disabled="empty"
          :aria-label="t('panels.console.actions.clear')"
          :title="t('panels.console.actions.clear')"
          @click="onClear"
        ><RotateCcw :size="14" /></button>
        <button
          type="button"
          class="monitor__btn"
          :class="{ 'is-flash': copyFlash }"
          :disabled="empty"
          :aria-label="copyFlash ? t('panels.console.actions.copied') : t('panels.console.actions.copy')"
          :title="copyFlash ? t('panels.console.actions.copied') : t('panels.console.actions.copy')"
          @click="onCopy"
        ><Clipboard :size="14" /></button>
      </div>
    </header>

    <div ref="scroller" class="monitor__screen">
      <div v-if="empty" class="monitor__empty">{{ t('panels.console.noOutput') }}</div>
      <article
        v-for="g in groups"
        :key="g.key"
        class="chunk"
        :class="`chunk--${g.kind}`"
        :style="{ '--chunk-color': colorOf(g.pid) }"
      >
        <header class="chunk__head">
          <span
            v-if="multicore && g.coreId !== undefined"
            class="chunk__core"
            :style="{ background: `var(--core-${g.coreId % 4})`, color: 'var(--core-fg)' }"
            :title="t('panels.console.coreOutTitle', { core: g.coreId })"
          >C{{ g.coreId }}</span>
          <span class="chunk__pid">PID {{ g.pid }}</span>
          <span class="chunk__sep">·</span>
          <span class="chunk__prog">
            {{ g.pid === 0 ? t('panels.console.kernelLabel') : g.prog }}
          </span>
        </header>
        <div class="chunk__row">
          <span
            class="chunk__tag"
            :class="`chunk__tag--${g.kind}`"
            :title="t(`panels.console.tag.${g.kind}Title`)"
          >{{ t(`panels.console.tag.${g.kind}`) }}</span>
          <pre class="chunk__body">{{ g.text }}</pre>
        </div>
        <pre v-if="showHex && g.bytes.length" class="chunk__hex">{{ hexOf(g.bytes) }}</pre>
      </article>
    </div>
  </section>
</template>

<style scoped>
.monitor {
  display: flex;
  flex-direction: column;
  background: var(--terminal-bg);
  border: 1px solid var(--terminal-border);
  border-radius: 0;
  box-shadow: inset 0 0 14px var(--surface-overlay-strong);
  position: relative;
  overflow: hidden;
  flex: 1;
  min-height: 0;
}

/* Local scanlines on top of the global ones, slightly more pronounced so the
   panel actually reads as a CRT and not a generic dark box. */
.monitor::before {
  content: '';
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: repeating-linear-gradient(
    0deg,
    transparent 0,
    color-mix(in oklab, var(--green) 4%, transparent) 1.5px,
    transparent 3px
  );
  opacity: 0.7;
  mix-blend-mode: screen;
}

.monitor__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 7px;
  height: 22px;
  flex-shrink: 0;
  border-bottom: 1px solid var(--terminal-border);
  background: color-mix(in oklab, var(--terminal-bg) 88%, var(--accent) 12%);
  z-index: 1;
  position: relative;
}

.monitor__title {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  color: var(--accent);
  letter-spacing: 0.8px;
  text-transform: uppercase;
  margin: 0;
}

.monitor__actions {
  display: flex;
  gap: 2px;
}

.monitor__btn {
  background: transparent;
  border: none;
  color: var(--text-dim);
  padding: 0;
  width: 18px;
  height: 18px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 0;
  cursor: pointer;
  transition: color 120ms ease, border-color 120ms ease, background 120ms ease;
}

.monitor__btn :deep(svg) {
  width: 12px;
  height: 12px;
  display: block;
}

.monitor__btn:hover:not(:disabled) {
  color: var(--green);
  border-color: var(--green);
}

.monitor__btn.is-active {
  color: var(--amber);
  border-color: var(--amber);
  background: color-mix(in oklab, var(--amber) 14%, transparent);
}

.monitor__btn.is-flash {
  color: var(--green);
  border-color: var(--green);
}

.monitor__btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.monitor__screen {
  flex: 1;
  overflow-y: auto;
  padding: 6px 8px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  position: relative;
  z-index: 0;
  min-height: 0;
}

.monitor__empty {
  color: var(--text-muted);
  font-style: italic;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  padding: 4px 2px;
}

.chunk {
  display: flex;
  flex-direction: column;
  gap: 2px;
  border-left: 2px solid var(--chunk-color);
  padding: 0 8px;
  animation: chunk-flash 600ms ease-out;
}

.chunk__head {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  color: var(--chunk-color);
  letter-spacing: 0.5px;
  text-transform: uppercase;
}

.chunk__pid {
  font-weight: 600;
}

.chunk__sep {
  color: var(--text-dim);
}

.chunk__prog {
  color: var(--text-dim);
  text-transform: none;
  letter-spacing: 0;
}

.chunk__core {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  font-weight: 700;
  padding: 0 4px;
  border-radius: 0;
  margin-right: 2px;
  text-transform: none;
}

.chunk__row {
  display: flex;
  align-items: baseline;
  gap: 8px;
}

.chunk__tag {
  flex-shrink: 0;
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.5px;
  text-transform: lowercase;
  line-height: 1.4;
  color: var(--chunk-color);
  opacity: 0.85;
  align-self: center;
}

.chunk__tag--in {
  color: var(--cyan);
}

.chunk__tag--out {
  color: var(--green);
}

.chunk__body {
  margin: 0;
  font-family: 'VT323', monospace;
  font-size: 16px;
  line-height: 1.1;
  color: var(--chunk-color);
  white-space: pre-wrap;
  word-break: break-all;
  text-shadow: 0 0 5px color-mix(in oklab, var(--chunk-color) 30%, transparent);
  flex: 1;
  min-width: 0;
}

.chunk__hex {
  margin: 2px 0 0 0;
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  color: var(--text-dim);
  /* The dump is pre-aligned column-by-column, so we explicitly forbid
     wrapping and letter-spacing — both would slide the ASCII column off
     position. Narrow viewports get a horizontal scroll on the block instead. */
  white-space: pre;
  word-break: normal;
  overflow-x: auto;
  border-top: 1px dashed var(--terminal-border);
  padding-top: 2px;
}

@keyframes chunk-flash {
  0% {
    background: color-mix(in oklab, var(--chunk-color) 18%, transparent);
  }
  100% {
    background: transparent;
  }
}

@media (max-width: 768px) {
  .chunk__body {
    font-size: 22px;
  }

  .monitor__screen {
    padding: 8px;
  }

  /* Shrink the title typography on phones so "CONSOLA · sys_write fd=1"
     fits in a single 32px header strip alongside the actions cluster
     instead of wrapping into a tall multi-line block. */
  .monitor__header {
    height: 30px;
    min-height: 30px;
    padding: 0 8px;
  }
  .monitor__title {
    font-size: 9px;
    letter-spacing: 0.5px;
    flex: 1 1 auto;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .monitor__actions { flex: 0 0 auto; gap: 2px; }
}

@media (max-width: 360px) {
  .chunk__body { font-size: 18px; }
  .monitor__screen { padding: 6px; }
}
</style>
