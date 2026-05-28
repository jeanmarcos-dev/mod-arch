<script setup lang="ts">
import { useCpuStore } from '../stores/cpu';
import { computed, onUpdated, ref } from 'vue';
import { useI18n } from 'vue-i18n';

const cpu = useCpuStore();
const scroller = ref<HTMLElement | null>(null);
const { t } = useI18n();

onUpdated(() => {
  if (scroller.value) scroller.value.scrollTop = scroller.value.scrollHeight;
});

function colorOf(c: string) {
  switch (c) {
    case 'green':  return 'var(--green)';
    case 'red':    return 'var(--red-soft)';
    case 'cyan':   return 'var(--cyan)';
    case 'yellow': return 'var(--yellow)';
    default:       return 'var(--amber)';
  }
}

// Hide log lines tagged with a core that the SimBar filter has hidden.
// Entries without a coreId (scheduler, syscalls, sync ops, session info)
// stay visible regardless of the filter.
const visibleLog = computed(() => cpu.log.filter(
  e => e.coreId === undefined || cpu.isCoreVisible(e.coreId),
));

// Only show per-core chips and tinted borders when the session is actually
// multicore. Single-core demos look exactly like before — no chip clutter
// for a system where every event is implicit C0.
const multicore = computed(() => cpu.cores.length > 1);
</script>

<template>
  <div class="panel log" ref="scroller" data-pizarra-anchor="events-panel">
    <div class="panel-title">{{ t('panels.events.title') }}</div>
    <div
      v-for="(e, i) in visibleLog"
      :key="i"
      :class="['line', { 'line--cored': multicore && e.coreId !== undefined }]"
      :style="multicore && e.coreId !== undefined
        ? { color: colorOf(e.color), borderLeftColor: `var(--core-${e.coreId % 4})` }
        : { color: colorOf(e.color) }"
    >
      <span
        v-if="multicore && e.coreId !== undefined"
        class="line__core-chip"
        :style="{ background: `var(--core-${e.coreId % 4})`, color: `var(--core-fg)` }"
        :title="t('panels.events.coreChip', { id: e.coreId })"
      >C{{ e.coreId }}</span>
      <span class="line__text">{{ e.text }}</span>
    </div>
    <div v-if="visibleLog.length === 0" class="empty">
      {{ cpu.log.length === 0
        ? t('panels.events.emptyLoad')
        : t('panels.events.emptyNoCores') }}
    </div>
  </div>
</template>

<style scoped>
.log {
  flex: 1;
  height: 100%;
  overflow-y: auto;
  font-size: 14px;
}

.line {
  padding: 1px 0;
  font-family: 'JetBrains Mono', monospace;
  white-space: pre;
}

/* Multicore rows wear a colored border-left so a quick scan reveals which
   core each event belongs to even when the chip text is clipped. The
   gutter has enough padding to keep the badge readable next to the text. */
.line--cored {
  border-left: 3px solid transparent;
  padding-left: 6px;
}

.line__core-chip {
  display: inline-block;
  min-width: 22px;
  padding: 0 4px;
  margin-right: 6px;
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0.5px;
  border-radius: var(--radius-sm);
  text-align: center;
  vertical-align: baseline;
}

.line__text {
  /* keep text on the same baseline as the chip without resetting white-space */
}

.empty {
  color: var(--text-muted);
  font-style: italic;
  padding: 20px 0;
  text-align: center;
}

@media (max-width: 768px) {
  .log {
    font-size: 13px;
  }

  .line {
    /* En mobile permitimos wrap para que los eventos largos no fuercen scroll horizontal. */
    white-space: pre-wrap;
    word-break: break-word;
  }
}
</style>
