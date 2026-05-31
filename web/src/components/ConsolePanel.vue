<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useCpuStore } from '../stores/cpu';
import ConsoleMonitor from './console/ConsoleMonitor.vue';
import KeyCap from './console/KeyCap.vue';
import KeyboardNarrator from './console/KeyboardNarrator.vue';

const cpu = useCpuStore();
const { t } = useI18n();

const keyInput = ref<HTMLInputElement | null>(null);
const lastChar = ref('');
const bumpSeq = ref(0);

interface SpecialKey {
  id: 'esc' | 'ctrlC' | 'enter' | 'backspace';
  code: string;
}
const SPECIAL_KEYS: SpecialKey[] = [
  { id: 'esc',       code: '\x1B' },
  { id: 'ctrlC',     code: '\x03' },
  { id: 'enter',     code: '\n'   },
  { id: 'backspace', code: '\x08' },
];

function sendKey(ch: string) {
  if (cpu.isViewer() || ch === '') return;
  cpu.pushKey(ch);
  lastChar.value = ch[0];
  bumpSeq.value++;
}

function onKeyPress(e: KeyboardEvent) {
  if (e.key.length === 1) {
    sendKey(e.key);
    e.preventDefault();
  } else if (e.key === 'Enter') {
    sendKey('\n');
    e.preventDefault();
  } else if (e.key === 'Backspace') {
    sendKey('\x08');
    e.preventDefault();
  }
}

function onKeyDown(e: KeyboardEvent) {
  // Pick up the special keys that don't fire keypress reliably across browsers.
  if (e.key === 'Escape') {
    sendKey('\x1B');
    e.preventDefault();
  }
}

function focusInput() {
  keyInput.value?.focus();
}

const kbufOverflow = computed(() => cpu.kbdBufUsed > cpu.kbdBufCap);

const kbufClass = computed(() => {
  if (cpu.kbdBufUsed === 0) return 'is-empty';
  if (kbufOverflow.value) return 'is-overflow';
  const fill = cpu.kbdBufUsed / Math.max(1, cpu.kbdBufCap);
  if (fill >= 0.95) return 'is-full';
  if (fill >= 0.5) return 'is-warm';
  return 'is-active';
});

const kbufLabel = computed(() => {
  if (kbufOverflow.value) {
    return t('panels.console.kbuf.labelOver', { used: cpu.kbdBufUsed, cap: cpu.kbdBufCap });
  }
  return t('panels.console.kbuf.label', { used: cpu.kbdBufUsed, cap: cpu.kbdBufCap });
});
</script>

<template>
  <div class="panel console" data-pizarra-anchor="console-panel">
    <div class="console__top">
      <ConsoleMonitor class="console__monitor" />
    </div>

    <div class="console__bottom">
      <section class="console__keyboard" :aria-label="t('panels.console.kbdAria')">
      <header class="kbd__header">
        <button
          type="button"
          class="kbd__title"
          :title="t('panels.console.kbdTooltip')"
          @click="cpu.openGlossary('IRQ 0x21')"
        >
          {{ t('panels.console.keyboardHw') }}
          <span class="kbd__title-hint">{{ t('panels.console.keyboardHint') }}</span>
        </button>
        <button
          type="button"
          class="kbd__kbuf"
          :class="kbufClass"
          :title="t('panels.console.kbuf.title', { used: cpu.kbdBufUsed, cap: cpu.kbdBufCap })"
          @click="cpu.openGlossary(t('panels.console.kbuf.glossary'))"
        >{{ kbufLabel }}</button>
      </header>

      <div class="kbd__readers">
        <span class="kbd__readers-label">{{ t('panels.console.readers.waiting') }}</span>
        <span v-if="cpu.kbdReaders.length === 0" class="kbd__readers-none">
          {{ t('panels.console.readers.none') }}
        </span>
        <ul v-else class="kbd__readers-list" role="list">
          <li
            v-for="r in cpu.kbdReaders"
            :key="r.pid"
            class="kbd__readers-item"
            role="listitem"
          >{{ r.prog
            ? t('panels.console.readers.item',        { pid: r.pid, prog: r.prog })
            : t('panels.console.readers.itemUnknown', { pid: r.pid }) }}</li>
        </ul>
      </div>

      <div class="kbd__interactor" @click="focusInput">
        <KeyCap :char="lastChar" :bump-seq="bumpSeq" />
        <div class="kbd__hint">
          <span v-if="cpu.isViewer()">{{ t('panels.console.placeholderViewer') }}</span>
          <span v-else>{{ t('panels.console.placeholderActive') }}</span>
        </div>
        <div
          class="kbd__specials"
          :aria-label="t('panels.console.specialKeys.ariaLabel')"
          @click.stop
        >
          <button
            v-for="k in SPECIAL_KEYS"
            :key="k.id"
            type="button"
            class="kbd__special"
            :disabled="cpu.isViewer()"
            :title="t(`panels.console.specialKeys.${k.id}Title`)"
            @click="sendKey(k.code)"
          >{{ t(`panels.console.specialKeys.${k.id}`) }}</button>
        </div>
        <input
          ref="keyInput"
          name="kbd_input"
          class="kbd__input"
          type="text"
          inputmode="text"
          autocomplete="off"
          autocapitalize="none"
          autocorrect="off"
          spellcheck="false"
          :disabled="cpu.isViewer()"
          :aria-label="t('panels.console.kbdAria')"
          @keypress="onKeyPress"
          @keydown="onKeyDown"
        />
      </div>
      </section>

      <KeyboardNarrator class="console__narrator" />
    </div>
  </div>
</template>

<style scoped>
.console {
  /* Height now comes from the parent ConsoleDrawer — the panel just fills
     it. Grid: stdout (CONSOLE) on top, KEYBOARD HW + NARRATOR sidebar at
     the bottom, mirroring a real terminal where output flows up and input
     lives below. */
  display: grid;
  grid-template-rows: 3fr 2fr;
  gap: 4px;
  height: 100%;
  min-height: 0;
  width: 100%;
}

.console__top {
  min-height: 0;
  min-width: 0;
  display: flex;
}

.console__bottom {
  min-height: 0;
  min-width: 0;
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(0, 1fr);
  gap: 4px;
}

.console__monitor,
.console__narrator {
  min-height: 0;
  min-width: 0;
}

.console__monitor {
  flex: 1 1 auto;
  width: 100%;
}

.console__keyboard {
  display: flex;
  flex-direction: column;
  gap: 4px;
  background: var(--panel-soft);
  border: 1px solid var(--border);
  border-radius: 0;
  padding: 5px 7px;
  min-height: 0;
}

.kbd__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 6px;
}

.kbd__title {
  background: transparent;
  border: 0;
  padding: 0;
  display: flex;
  align-items: baseline;
  gap: 5px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  color: var(--green);
  letter-spacing: 0.8px;
  text-transform: uppercase;
  cursor: pointer;
  justify-self: flex-start;
  min-height: auto;
}

.kbd__title:hover {
  color: var(--green-glow, var(--green));
  text-shadow: 0 0 6px color-mix(in oklab, var(--green) 50%, transparent);
}

.kbd__title-hint {
  font-size: 9px;
  font-style: italic;
  color: var(--text-dim);
  letter-spacing: 0;
  text-transform: none;
}

.kbd__kbuf {
  background: transparent;
  border: 1px solid var(--terminal-border);
  border-radius: 0;
  padding: 1px 7px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 9px;
  color: var(--text-dim);
  cursor: pointer;
  transition: color 120ms ease, border-color 120ms ease, box-shadow 120ms ease;
}

.kbd__kbuf.is-empty {
  color: var(--text-dim);
}

.kbd__kbuf.is-active {
  color: var(--green);
  border-color: var(--green);
  box-shadow: 0 0 6px color-mix(in oklab, var(--green) 35%, transparent);
}

.kbd__kbuf.is-warm {
  color: var(--amber);
  border-color: var(--amber);
  box-shadow: 0 0 6px color-mix(in oklab, var(--amber) 40%, transparent);
}

.kbd__kbuf.is-full,
.kbd__kbuf.is-overflow {
  color: var(--red, #ff6b6b);
  border-color: var(--red, #ff6b6b);
  box-shadow: 0 0 8px color-mix(in oklab, var(--red, #ff6b6b) 50%, transparent);
  animation: kbuf-pulse 700ms ease-in-out infinite alternate;
}

@keyframes kbuf-pulse {
  from { opacity: 1; }
  to   { opacity: 0.55; }
}

.kbd__readers {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 5px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 9px;
  color: var(--text-dim);
  padding: 3px 2px 0;
  border-top: 1px dashed var(--border);
  min-height: 12px;
}

.kbd__readers-label {
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.kbd__readers-none {
  font-style: italic;
  color: var(--text-muted);
}

.kbd__readers-list {
  list-style: none;
  display: flex;
  flex-wrap: wrap;
  gap: 4px 10px;
  margin: 0;
  padding: 0;
}

.kbd__readers-item {
  color: var(--cyan);
  background: color-mix(in oklab, var(--cyan) 12%, transparent);
  border-radius: 0;
  padding: 1px 6px;
}

.kbd__interactor {
  display: flex;
  align-items: center;
  gap: 8px;
  background: var(--terminal-bg);
  border: 1px solid var(--terminal-border);
  border-radius: 0;
  padding: 3px 6px;
  cursor: text;
  position: relative;
}

.kbd__interactor:focus-within {
  border-color: var(--green);
  box-shadow: 0 0 6px color-mix(in oklab, var(--green) 40%, transparent);
}

.kbd__hint {
  flex: 1;
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  font-style: italic;
  color: var(--text-muted);
  line-height: 1.25;
}

.kbd__specials-row {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

/* Input is the focus carrier — visually invisible but kept on-screen and
   tab-accessible for keyboard users and screen readers. */
.kbd__input {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  background: transparent;
  border: 0;
  outline: none;
  padding: 0;
  margin: 0;
  color: transparent;
  caret-color: transparent;
  font-size: 1px;
}

.kbd__specials {
  display: flex;
  gap: 4px;
  flex-shrink: 0;
  position: relative;
  z-index: 1;
}

.kbd__special {
  background: var(--terminal-bg);
  border: 1px solid var(--terminal-border);
  color: var(--text-dim);
  font-family: 'JetBrains Mono', monospace;
  font-size: 9px;
  padding: 1px 5px;
  border-radius: 0;
  cursor: pointer;
  line-height: 1.3;
  white-space: nowrap;
  transition: color 120ms ease, border-color 120ms ease, background 120ms ease;
}

.kbd__special:hover:not(:disabled) {
  color: var(--green);
  border-color: var(--green);
  background: color-mix(in oklab, var(--green) 10%, transparent);
}

.kbd__special:active:not(:disabled) {
  transform: translateY(1px);
}

.kbd__special:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

@media (max-width: 768px) {
  .console {
    /* Mobile shows the drawer as a full-viewport tab; stack everything in a
       single column so the monitor, the keyboard and the narrator all keep
       a readable strip without horizontal squeeze. */
    grid-template-rows: auto auto auto;
    grid-auto-flow: row;
    overflow-y: auto;
  }

  .console__bottom {
    grid-template-columns: 1fr;
    grid-template-rows: auto auto;
  }

  .console__monitor {
    min-height: 140px;
    max-height: 28vh;
  }
  .console__narrator { max-height: 28vh; }

  .console__keyboard {
    flex: 0 0 auto;
  }

  .kbd__hint {
    font-size: 12px;
    min-width: 0;
  }

  .kbd__interactor {
    flex-wrap: wrap;
    row-gap: 6px;
    padding: 6px 8px;
  }

  /* Specials sit in their own row, wrap into a clean grid so ESC/CTRL+C/
     ENTER/BKSP keep a comfortable tap target each instead of stacking
     into a column. */
  .kbd__specials {
    width: 100%;
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 4px;
    justify-content: stretch;
  }

  .kbd__special {
    font-size: 11px;
    padding: 6px 4px;
    height: auto;
    min-height: auto;
    width: 100%;
  }
}

@media (max-width: 360px) {
  .console { grid-template-rows: minmax(140px, 1fr) auto auto; }
  .console__monitor { min-height: 140px; max-height: 36vh; }
  .kbd__specials { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .kbd__hint { font-size: 11px; }
}
</style>
