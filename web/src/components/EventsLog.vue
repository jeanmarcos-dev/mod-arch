<script setup lang="ts">
import { useCpuStore } from '../stores/cpu';
import { computed, onUpdated, ref } from 'vue';

const cpu = useCpuStore();
const scroller = ref<HTMLElement | null>(null);

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
</script>

<template>
  <div class="panel log" ref="scroller" data-pizarra-anchor="events-panel">
    <div class="panel-title">EVENT_LOG</div>
    <div v-for="(e, i) in visibleLog" :key="i" class="line" :style="{ color: colorOf(e.color) }">
      {{ e.text }}
    </div>
    <div v-if="visibleLog.length === 0" class="empty">
      {{ cpu.log.length === 0
        ? 'Cargue un programa y presione STEP para ver eventos.'
        : 'Sin eventos para los núcleos visibles. Active más núcleos en la SimBar.' }}
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
