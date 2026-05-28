<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { MemoryOwner } from '../../lib/memoryOwners';
import { fmtAddr, coreColorVar, coreGlowVar } from '../../lib/memoryLayout';
import Term from '../glossary/Term.vue';

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
const codeEnd = computed(() => props.owner.codeBase + Math.max(0, props.owner.codeSize - 1));
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
  <div
    :class="['owner-chip', {
      'owner-chip--kernel': isKernel,
      'owner-chip--process': !isKernel,
      'owner-chip--running': owner.runningOnCore !== undefined,
      'owner-chip--thread': owner.isThread,
      'owner-chip--selected': selected,
    }]"
    :style="styleVars"
    :data-owner-id="owner.id"
    @click="emit('select', owner.id)"
  >
    <header class="owner-chip__head">
      <span class="owner-chip__id">
        <template v-if="isKernel">
          <span class="owner-chip__id-k">{{ t('shell.memoryOwnerChip.kernelLabel') }}</span>
        </template>
        <template v-else>
          <Term term="PID" mode="label" class="owner-chip__id-k">PID</Term>
          <span class="owner-chip__id-v">{{ owner.pid }}</span>
        </template>
      </span>
      <span class="owner-chip__name">{{ owner.name }}</span>
      <span
        v-if="owner.isThread && owner.tgid"
        class="owner-chip__thread"
        :title="t('shell.memoryOwnerChip.threadHint', { tgid: owner.tgid })"
      >TID</span>
    </header>

    <dl class="owner-chip__ranges">
      <div
        class="owner-chip__range owner-chip__range--clickable"
        :title="t('shell.memoryOwnerChip.jumpCodeHint')"
        @click.stop="onJumpCode"
      >
        <dt>{{ t('shell.memoryOwnerChip.codeLabel') }}</dt>
        <dd>{{ fmtAddr(owner.codeBase) }} <span class="owner-chip__range-end">→ {{ fmtAddr(codeEnd) }}</span></dd>
      </div>
      <div
        v-if="!owner.isThread"
        class="owner-chip__range owner-chip__range--clickable"
        :title="t('shell.memoryOwnerChip.jumpStackHint')"
        @click.stop="onJumpStack"
      >
        <dt>{{ t('shell.memoryOwnerChip.stackLabel') }}</dt>
        <dd>{{ fmtAddr(owner.stackTop) }}</dd>
      </div>
    </dl>

    <footer v-if="owner.runningOnCore !== undefined || owner.state" class="owner-chip__foot">
      <span
        v-if="owner.runningOnCore !== undefined"
        class="owner-chip__running"
      >{{ t('shell.memoryOwnerChip.runningOn', { id: owner.runningOnCore }) }}</span>
      <span
        v-if="owner.state"
        :class="['owner-chip__state', `owner-chip__state--${owner.state.toLowerCase()}`]"
      >{{ owner.state }}</span>
    </footer>
  </div>
</template>

<style scoped>
.owner-chip {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  background: var(--surface-2);
  cursor: pointer;
  transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
}

.owner-chip:hover {
  border-color: var(--border-strong);
}

.owner-chip--kernel {
  border-color: color-mix(in oklab, var(--red-soft) 45%, transparent);
  background: color-mix(in oklab, var(--red-soft) 7%, var(--surface-2));
}

.owner-chip--kernel:hover {
  border-color: var(--red-soft);
}

.owner-chip--running {
  border-left: 3px solid var(--core-color, var(--amber));
}

.owner-chip--running::after {
  content: '';
  position: absolute;
  right: 8px;
  top: 10px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--core-color, var(--amber));
  box-shadow: var(--core-glow, 0 0 6px var(--amber));
}

.owner-chip--thread {
  border-style: dashed;
}

.owner-chip--selected {
  box-shadow: var(--core-glow, 0 0 0 1px var(--amber));
  border-color: var(--core-color, var(--amber));
}

.owner-chip__head {
  display: flex;
  align-items: baseline;
  gap: 6px;
  flex-wrap: wrap;
}

.owner-chip__id {
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
  font-family: var(--font-display);
}

.owner-chip__id-k {
  color: var(--text-muted);
  font-size: 10px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
}

.owner-chip__id-v {
  color: var(--amber);
  font-family: 'JetBrains Mono', monospace;
  font-size: 14px;
}

.owner-chip--kernel .owner-chip__id-k {
  color: var(--red-soft);
  font-size: 13px;
  letter-spacing: 3px;
  text-shadow: var(--shadow-glow-red, none);
}

.owner-chip__name {
  color: var(--text);
  font-size: 11px;
  font-family: 'JetBrains Mono', monospace;
  word-break: break-word;
}

.owner-chip__thread {
  font-size: 8.5px;
  letter-spacing: 1px;
  color: var(--text-muted);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 0 4px;
}

.owner-chip__ranges {
  display: grid;
  gap: 2px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 10.5px;
  margin: 0;
}

.owner-chip__range {
  display: grid;
  grid-template-columns: 44px 1fr;
  gap: var(--space-2);
  align-items: baseline;
  padding: 2px 4px;
  border-radius: var(--radius-sm);
}

.owner-chip__range dt {
  color: var(--text-muted);
  letter-spacing: 1px;
  font-size: 9px;
  text-transform: uppercase;
}

.owner-chip__range dd {
  margin: 0;
  color: var(--text);
  text-align: right;
  white-space: nowrap;
}

.owner-chip__range-end {
  color: var(--text-muted);
  font-size: 9.5px;
}

.owner-chip__range--clickable {
  cursor: pointer;
  transition: background var(--transition-fast), color var(--transition-fast);
}

.owner-chip__range--clickable:hover {
  background: color-mix(in oklab, var(--amber) 12%, transparent);
}

.owner-chip__range--clickable:hover dd {
  color: var(--amber);
}

.owner-chip__foot {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 9.5px;
  letter-spacing: 1px;
  color: var(--text-muted);
  text-transform: uppercase;
  border-top: 1px dashed var(--border);
  padding-top: 4px;
}

.owner-chip__running {
  color: var(--core-color, var(--amber));
  text-shadow: var(--core-glow, none);
  font-family: 'JetBrains Mono', monospace;
}

.owner-chip__state {
  margin-left: auto;
  padding: 0 4px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border);
}

.owner-chip__state--running   { color: var(--green); border-color: var(--green); }
.owner-chip__state--ready     { color: var(--cyan);  border-color: var(--cyan);  }
.owner-chip__state--blocked   { color: var(--yellow); border-color: var(--yellow); }
.owner-chip__state--zombie    { color: var(--red-soft); border-color: var(--red-soft); }
.owner-chip__state--terminated{ color: var(--text-dim); border-color: var(--text-dim); }
</style>
