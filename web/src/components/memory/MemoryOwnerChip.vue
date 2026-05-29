<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { ChevronDown, ChevronRight } from '@lucide/vue';
import type { MemoryOwner } from '../../lib/memoryOwners';
import { OWNER_DEPTH_CAP } from '../../lib/memoryOwners';
import { fmtAddr, coreColorVar, coreGlowVar } from '../../lib/memoryLayout';

const { t } = useI18n();

const props = defineProps<{
  owner: MemoryOwner;
  selected?: boolean;
}>();

const emit = defineEmits<{
  (e: 'jump-code', addr: number): void;
  (e: 'jump-stack', addr: number): void;
  (e: 'select', id: string): void;
}>();

const isKernel = computed(() => props.owner.kind === 'kernel');
// Stack hint: jumping right at stack_top usually shows empty bytes since the
// stack grows down. Land a window above so the user sees recently pushed
// data instead of zeros.
const stackJumpAddr = computed(() => Math.max(0, (props.owner.stackTop & ~0xff) - 0x80));

const hasThreads = computed(() => props.owner.threads.length > 0);
// Default: expand when there are few threads so the relationship is visible
// at a glance; collapse on larger groups to keep the sidebar scannable.
const threadsExpanded = ref(props.owner.threads.length > 0 && props.owner.threads.length <= 2);

const styleVars = computed(() => {
  const id = props.owner.runningOnCore;
  const vars: Record<string, string> = {
    '--owner-depth': String(props.owner.depth),
  };
  if (id !== undefined) {
    vars['--core-color'] = coreColorVar(id);
    vars['--core-glow'] = coreGlowVar(id);
  }
  return vars;
});

const showParentChip = computed(() => {
  const pp = props.owner.parentPid;
  return pp !== undefined && pp > 0;
});

const showDepthChip = computed(() => props.owner.realDepth > OWNER_DEPTH_CAP);

function onJumpCode() { emit('jump-code', props.owner.codeBase); }
function onJumpStack() { emit('jump-stack', stackJumpAddr.value); }
function toggleThreads() { threadsExpanded.value = !threadsExpanded.value; }

function threadStateClass(state: string): string {
  return `owner-thread__state--${state.toLowerCase()}`;
}
</script>

<template>
  <article
    :class="['owner-card', {
      'owner-card--kernel': isKernel,
      'owner-card--running': owner.runningOnCore !== undefined,
      'owner-card--selected': selected,
      'owner-card--nested': owner.depth > 0,
    }]"
    :style="styleVars"
    :data-owner-id="owner.id"
    @click="emit('select', owner.id)"
  >
    <header class="owner-card__head">
      <span v-if="isKernel" class="owner-card__title owner-card__title--kernel">{{ t('shell.memoryOwnerChip.kernelLabel') }}</span>
      <span v-else class="owner-card__title">
        <span class="owner-card__pid">P{{ owner.pid }}</span>
        <span class="owner-card__name">{{ owner.name }}</span>
      </span>
      <span
        v-if="showParentChip"
        class="owner-card__chip owner-card__chip--parent"
        :title="t('shell.memoryOwnerChip.parentChip', { pid: owner.parentPid })"
      >↑P{{ owner.parentPid }}</span>
      <span
        v-if="showDepthChip"
        class="owner-card__chip owner-card__chip--depth"
        :title="t('shell.memoryOwnerChip.depthChip', { depth: owner.realDepth })"
      >↳d{{ owner.realDepth }}</span>
      <span
        v-if="owner.state"
        :class="['owner-card__state', `owner-card__state--${owner.state.toLowerCase()}`]"
      >{{ owner.state }}</span>
      <span
        v-else-if="isKernel && owner.runningOnCore !== undefined"
        class="owner-card__state owner-card__state--running"
      >C{{ owner.runningOnCore }}</span>
    </header>

    <div class="owner-card__body">
      <button
        type="button"
        class="owner-card__row owner-card__row--clickable"
        :title="t('shell.memoryOwnerChip.jumpCodeHint')"
        @click.stop="onJumpCode"
      >
        <span class="owner-card__row-k">{{ t('shell.memoryOwnerChip.codeLabel') }}</span>
        <span class="owner-card__row-v">{{ fmtAddr(owner.codeBase) }}</span>
      </button>
      <button
        type="button"
        class="owner-card__row owner-card__row--clickable"
        :title="t('shell.memoryOwnerChip.jumpStackHint')"
        @click.stop="onJumpStack"
      >
        <span class="owner-card__row-k">{{ t('shell.memoryOwnerChip.stackLabel') }}</span>
        <span class="owner-card__row-v">{{ fmtAddr(owner.stackTop) }}</span>
      </button>
    </div>

    <section v-if="hasThreads" class="owner-card__threads">
      <button
        type="button"
        class="owner-card__threads-toggle"
        :aria-expanded="threadsExpanded"
        :title="threadsExpanded
          ? t('shell.memoryOwnerChip.threadsCollapse')
          : t('shell.memoryOwnerChip.threadsExpand')"
        @click.stop="toggleThreads"
      >
        <component
          :is="threadsExpanded ? ChevronDown : ChevronRight"
          :size="11"
          :stroke-width="2"
          class="owner-card__threads-chevron"
        />
        <span class="owner-card__threads-label">{{ t('shell.memoryOwnerChip.threadsTitle', { n: owner.threads.length }) }}</span>
      </button>
      <ul v-if="threadsExpanded" class="owner-card__threads-list">
        <li
          v-for="thr in owner.threads"
          :key="thr.pid"
          class="owner-thread"
          :title="t('shell.memoryOwnerChip.threadShares', { leader: owner.pid })"
        >
          <span class="owner-thread__id">P{{ thr.pid }}</span>
          <span class="owner-thread__tid">t{{ thr.tid ?? thr.pid }}</span>
          <span class="owner-thread__name">{{ thr.name }}</span>
          <span
            v-if="thr.state"
            :class="['owner-thread__state', threadStateClass(thr.state)]"
          >{{ thr.state }}</span>
          <span
            v-if="thr.runningOnCore !== undefined"
            class="owner-thread__core"
            :style="{ color: `var(--core-${thr.runningOnCore % 4})` }"
          >{{ t('shell.memoryOwnerChip.threadOn', { id: thr.runningOnCore }) }}</span>
        </li>
      </ul>
    </section>

    <footer
      v-if="!isKernel && owner.runningOnCore !== undefined"
      class="owner-card__foot"
    >{{ t('shell.memoryOwnerChip.runningOn', { id: owner.runningOnCore }) }}</footer>
  </article>
</template>

<style scoped>
/* Compact PCB-style card. Mirrors the look of ProcessesPanel cards but
   strips everything not relevant to memory (registers, EFLAGS, tree). The
   container is `min-width: 0` and every text node clips with ellipsis so
   the card never widens past the narrow memory sidebar. */
.owner-card {
  position: relative;
  display: flex;
  flex-direction: column;
  min-width: 0;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--border);
  border-left: 3px solid var(--core-color, var(--border-strong));
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  cursor: pointer;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
  /* Indent driven by the fork-tree depth. Capped in TS to OWNER_DEPTH_CAP so
     deeply nested chains never push the chip off-screen. */
  margin-left: calc(var(--owner-depth, 0) * 10px);
}

.owner-card:hover {
  border-color: var(--border-strong);
  border-left-color: var(--core-color, var(--amber));
}

/* Subtle vertical guideline on nested cards, echoing the indent in
   ProcessesPanel so the two views feel like the same family. */
.owner-card--nested::before {
  content: '';
  position: absolute;
  left: -6px;
  top: 50%;
  width: 4px;
  height: 1px;
  background: var(--border);
}

.owner-card--kernel {
  border-color: color-mix(in oklab, var(--red-soft) 35%, var(--border));
  border-left-color: var(--red-soft);
  background: color-mix(in oklab, var(--red-soft) 6%, var(--surface-2));
}

.owner-card--running::after {
  content: '';
  position: absolute;
  right: 6px;
  top: 8px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--core-color, var(--amber));
  box-shadow: var(--core-glow, 0 0 6px var(--amber));
}

.owner-card--selected {
  box-shadow: 0 0 0 1px var(--core-color, var(--amber)), var(--core-glow, none);
}

.owner-card__head {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  min-width: 0;
  flex-wrap: wrap;
  row-gap: 2px;
}

.owner-card__title {
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
  min-width: 0;
  flex: 1;
  overflow: hidden;
}

.owner-card__pid {
  color: var(--amber);
  font-weight: 700;
  flex-shrink: 0;
}

.owner-card__name {
  color: var(--text);
  font-size: 10.5px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}

.owner-card__title--kernel {
  color: var(--red-soft);
  font-family: var(--font-display);
  letter-spacing: 3px;
  font-size: 12px;
  text-transform: uppercase;
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.owner-card__state {
  font-size: 9px;
  letter-spacing: 1px;
  padding: 0 5px;
  border: 1px solid;
  border-radius: var(--radius-sm);
  text-transform: uppercase;
  flex-shrink: 0;
}

.owner-card__state--running   { color: var(--green);    border-color: var(--green);    }
.owner-card__state--ready     { color: var(--cyan);     border-color: var(--cyan);     }
.owner-card__state--blocked   { color: var(--yellow);   border-color: var(--yellow);   }
.owner-card__state--zombie    { color: var(--red-soft); border-color: var(--red-soft); }
.owner-card__state--terminated{ color: var(--text-dim); border-color: var(--text-dim); }
.owner-card__state--new       { color: var(--text-dim); border-color: var(--text-dim); }

.owner-card__chip {
  font-size: 8.5px;
  letter-spacing: 0.5px;
  padding: 0 4px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  background: transparent;
  flex-shrink: 0;
}

.owner-card__chip--parent {
  color: var(--text-dim);
  border-color: var(--border);
}

.owner-card__chip--depth {
  color: var(--text-muted);
  border-style: dashed;
}

.owner-card__body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.owner-card__row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--space-2);
  padding: 1px 2px;
  background: transparent;
  border: none;
  border-radius: 2px;
  color: inherit;
  font-family: inherit;
  font-size: 10.5px;
  min-width: 0;
  min-height: unset;
  text-align: left;
}

.owner-card__row-k {
  color: var(--text-muted);
  font-size: 9px;
  letter-spacing: 1px;
  text-transform: uppercase;
  flex-shrink: 0;
}

.owner-card__row-v {
  color: var(--text);
  font-family: 'JetBrains Mono', monospace;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}

.owner-card__row--clickable {
  cursor: pointer;
  transition: background var(--transition-fast), color var(--transition-fast);
}

.owner-card__row--clickable:hover {
  background: color-mix(in oklab, var(--amber) 14%, transparent);
}

.owner-card__row--clickable:hover .owner-card__row-v {
  color: var(--amber);
}

.owner-card__foot {
  font-size: 9px;
  letter-spacing: 1px;
  text-transform: uppercase;
  color: var(--core-color, var(--amber));
  text-shadow: var(--core-glow, none);
  border-top: 1px dashed var(--border);
  padding-top: 3px;
}

/* ─── Threads section (collapsible) ─────────────────────────────────────── */

.owner-card__threads {
  display: flex;
  flex-direction: column;
  gap: 2px;
  border-top: 1px dashed var(--border);
  padding-top: 4px;
}

.owner-card__threads-toggle {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: transparent;
  border: none;
  padding: 1px 2px;
  color: var(--text-muted);
  font-family: inherit;
  font-size: 9px;
  letter-spacing: 1px;
  text-transform: uppercase;
  cursor: pointer;
  min-height: unset;
  text-align: left;
  transition: color var(--transition-fast);
}

.owner-card__threads-toggle:hover {
  color: var(--amber);
}

.owner-card__threads-chevron {
  flex-shrink: 0;
}

.owner-card__threads-list {
  list-style: none;
  margin: 0;
  padding: 0 0 0 var(--space-2);
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.owner-thread {
  display: flex;
  align-items: baseline;
  gap: 4px;
  font-size: 10px;
  padding: 1px 2px;
  border-radius: 2px;
  min-width: 0;
}

.owner-thread:hover {
  background: color-mix(in oklab, var(--amber) 8%, transparent);
}

.owner-thread__id {
  color: var(--amber);
  font-weight: 600;
  flex-shrink: 0;
}

.owner-thread__tid {
  color: var(--text-muted);
  font-size: 9px;
  flex-shrink: 0;
}

.owner-thread__name {
  color: var(--text);
  font-size: 9.5px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
  flex: 1;
}

.owner-thread__state {
  font-size: 8px;
  letter-spacing: 1px;
  padding: 0 3px;
  border: 1px solid;
  border-radius: 2px;
  text-transform: uppercase;
  flex-shrink: 0;
}

.owner-thread__state--running    { color: var(--green);    border-color: var(--green);    }
.owner-thread__state--ready      { color: var(--cyan);     border-color: var(--cyan);     }
.owner-thread__state--blocked    { color: var(--yellow);   border-color: var(--yellow);   }
.owner-thread__state--zombie     { color: var(--red-soft); border-color: var(--red-soft); }
.owner-thread__state--terminated { color: var(--text-dim); border-color: var(--text-dim); }
.owner-thread__state--new        { color: var(--text-dim); border-color: var(--text-dim); }

.owner-thread__core {
  font-size: 8.5px;
  letter-spacing: 0.5px;
  flex-shrink: 0;
  font-family: 'JetBrains Mono', monospace;
}
</style>
