<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type {
  VarMeta,
  VarMetaGlobal,
  VarMetaLocal,
  VarSnapshot,
  GlobalSnapshot,
  ThreadVarSnapshot,
  LocalSnapshot,
} from '../stores/cpu';

const props = defineProps<{
  name: string;
  anchorRect: DOMRect | null;
  varMeta: VarMeta | null;
  varSnapshot: VarSnapshot | null;
}>();

const emit = defineEmits<{
  (e: 'stay'): void;
  (e: 'leave'): void;
}>();

const { t } = useI18n();

// Classify the hovered token: global → show one row with its addr/value;
// local → show a row per live thread that has it in scope. If both globals
// and locals share the name (shouldn't happen in well-formed demos), we
// prefer the global to mirror typical C scoping rules.
const matchedGlobal = computed<VarMetaGlobal | null>(() => {
  const meta = props.varMeta;
  if (!meta?.globals) return null;
  return meta.globals.find(g => g.name === props.name) ?? null;
});

const matchedLocals = computed<VarMetaLocal[]>(() => {
  const meta = props.varMeta;
  if (!meta?.locals) return [];
  return meta.locals.filter(l => l.name === props.name);
});

const globalLive = computed<GlobalSnapshot | null>(() => {
  if (!matchedGlobal.value || !props.varSnapshot) return null;
  return props.varSnapshot.globals.find(g => g.name === props.name) ?? null;
});

interface LocalLiveRow {
  thread: ThreadVarSnapshot;
  local: LocalSnapshot | null;
}
const localLiveRows = computed<LocalLiveRow[]>(() => {
  if (matchedLocals.value.length === 0 || !props.varSnapshot) return [];
  // Only threads currently in scope of a function that owns this local.
  const funcs = new Set(matchedLocals.value.map(l => l.func));
  const rows: LocalLiveRow[] = [];
  for (const th of props.varSnapshot.threads) {
    if (!th.func || !funcs.has(th.func)) continue;
    const local = th.locals.find(l => l.name === props.name) ?? null;
    rows.push({ thread: th, local });
  }
  return rows;
});

// Position the popover anchored to the rectangle of the hovered span. The
// fixed coordinates keep the box in viewport space, which dodges any
// scrollable ancestor's transform.
const style = computed(() => {
  const r = props.anchorRect;
  if (!r) return { display: 'none' };
  const top = Math.max(8, r.top - 8);
  // Place the popover to the right of the token, falling back to the left
  // if there's no room.
  const overflowRight = r.right + 280 > window.innerWidth;
  if (overflowRight) {
    return {
      position: 'fixed' as const,
      top: `${top}px`,
      left: `${Math.max(8, r.left - 280)}px`,
    };
  }
  return {
    position: 'fixed' as const,
    top: `${top}px`,
    left: `${r.right + 8}px`,
  };
});

function locLabel(l: { kind: string; reg?: string; offset?: number; addr?: number }): string {
  if (l.kind === 'reg') return `[${l.reg}]`;
  if (l.kind === 'stack') {
    const off = l.offset ?? 0;
    if (off === 0) return '[EBP]';
    if (off < 0) return `[EBP${off}]`;
    return `[EBP+${off}]`;
  }
  if (l.kind === 'mem' && l.addr !== undefined) {
    return `[0x${l.addr.toString(16).toUpperCase()}]`;
  }
  return '';
}
</script>

<template>
  <div
    class="var-popover"
    :style="style"
    @mouseenter="emit('stay')"
    @mouseleave="emit('leave')"
  >
    <header class="vp-head">
      <span class="vp-name">{{ name }}</span>
      <span v-if="matchedGlobal" class="vp-scope">{{ t('panels.codeAsm.popoverScopeGlobal') }}</span>
      <span v-else-if="matchedLocals.length > 0" class="vp-scope">{{ t('panels.codeAsm.popoverScopeLocal') }}</span>
    </header>

    <!-- Global path: single row with the live value. -->
    <div v-if="matchedGlobal" class="vp-row">
      <span class="vp-type">{{ matchedGlobal.type }}</span>
      <span class="vp-loc">{{ matchedGlobal.addr }}</span>
      <span v-if="globalLive?.unmapped" class="vp-val vp-val--missing">{{ t('panels.codeAsm.unmapped') }}</span>
      <span v-else-if="globalLive" class="vp-val">
        {{ globalLive.value_dec }}
        <span class="vp-val-hex">{{ globalLive.value_hex }}</span>
      </span>
      <span v-else class="vp-val vp-val--missing">…</span>
    </div>

    <!-- Local path: one row per thread that has this local in scope. -->
    <div v-else-if="matchedLocals.length > 0">
      <div v-if="localLiveRows.length === 0" class="vp-empty">
        {{ t('panels.codeAsm.popoverNoLiveThread') }}
      </div>
      <ul v-else class="vp-thread-list">
        <li v-for="row in localLiveRows" :key="row.thread.tid" class="vp-thread-row">
          <span class="vp-tlabel">{{ row.thread.label }}</span>
          <span class="vp-loc" v-if="row.local">{{ locLabel(row.local.loc) }}</span>
          <span class="vp-val vp-val--missing" v-if="!row.local">{{ t('panels.codeAsm.unset') }}</span>
          <span class="vp-val vp-val--missing" v-else-if="row.local.unset">{{ t('panels.codeAsm.unset') }}</span>
          <span class="vp-val vp-val--missing" v-else-if="row.local.unmapped">{{ t('panels.codeAsm.unmapped') }}</span>
          <span class="vp-val" v-else>
            {{ row.local.value_dec }}
            <span class="vp-val-hex">{{ row.local.value_hex }}</span>
          </span>
        </li>
      </ul>
    </div>

    <div v-else class="vp-empty">{{ t('panels.codeAsm.popoverUnknown') }}</div>
  </div>
</template>

<style scoped>
.var-popover {
  z-index: 9999;
  /* Solid background — a tooltip that reveals the C source behind it makes
     the values harder to read. --bg-1 matches the panel surface. */
  background: var(--bg-1);
  border: 1px solid var(--amber);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.6);
  padding: 8px 10px;
  min-width: 200px;
  max-width: 320px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  line-height: 1.45;
  pointer-events: auto;
}
.vp-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  margin-bottom: 4px;
  padding-bottom: 4px;
  border-bottom: 1px dashed var(--border);
}
.vp-name { color: var(--amber); font-weight: 600; }
.vp-scope {
  color: var(--text-muted);
  font-family: var(--font-display);
  font-size: 9px;
  letter-spacing: 2px;
  text-transform: uppercase;
}
.vp-row {
  display: grid;
  grid-template-columns: 32px auto 1fr;
  gap: 6px;
  align-items: baseline;
}
.vp-type { color: var(--amber-dim); font-size: 10px; text-transform: uppercase; letter-spacing: 1px; }
.vp-loc { color: var(--text-muted); font-size: 10px; }
.vp-val { color: var(--text); justify-self: end; }
.vp-val-hex { color: var(--text-muted); font-size: 10px; margin-left: 4px; }
.vp-val--missing { color: var(--text-muted); font-style: italic; }
.vp-thread-list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.vp-thread-row {
  display: grid;
  grid-template-columns: 32px auto 1fr;
  gap: 6px;
  align-items: baseline;
  padding: 2px 0;
}
.vp-tlabel {
  color: var(--text);
  font-family: var(--font-display);
  font-size: 9px;
  letter-spacing: 1px;
}
.vp-empty {
  color: var(--text-muted);
  font-style: italic;
  font-size: 10px;
}
</style>
