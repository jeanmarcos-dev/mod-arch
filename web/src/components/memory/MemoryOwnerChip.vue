<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { MemoryOwner } from '../../lib/memoryOwners';
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

const styleVars = computed(() => {
  const id = props.owner.runningOnCore;
  if (id === undefined) return {};
  return {
    '--core-color': coreColorVar(id),
    '--core-glow': coreGlowVar(id),
  };
});

function onJumpCode() { emit('jump-code', props.owner.codeBase); }
function onJumpStack() { emit('jump-stack', stackJumpAddr.value); }
</script>

<template>
  <article
    :class="['owner-card', {
      'owner-card--kernel': isKernel,
      'owner-card--running': owner.runningOnCore !== undefined,
      'owner-card--thread': owner.isThread,
      'owner-card--selected': selected,
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
        v-if="!owner.isThread"
        type="button"
        class="owner-card__row owner-card__row--clickable"
        :title="t('shell.memoryOwnerChip.jumpStackHint')"
        @click.stop="onJumpStack"
      >
        <span class="owner-card__row-k">{{ t('shell.memoryOwnerChip.stackLabel') }}</span>
        <span class="owner-card__row-v">{{ fmtAddr(owner.stackTop) }}</span>
      </button>
    </div>

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
}

.owner-card:hover {
  border-color: var(--border-strong);
  border-left-color: var(--core-color, var(--amber));
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

.owner-card--thread {
  border-style: dashed;
}

.owner-card--selected {
  box-shadow: 0 0 0 1px var(--core-color, var(--amber)), var(--core-glow, none);
}

.owner-card__head {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  min-width: 0;
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
</style>
