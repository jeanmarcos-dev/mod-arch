<script setup lang="ts">
import { useCpuStore } from '../stores/cpu';
import { onUpdated, ref } from 'vue';

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
</script>

<template>
  <div class="panel log" ref="scroller">
    <div class="panel-title">EVENT_LOG</div>
    <div v-for="(e, i) in cpu.log" :key="i" class="line" :style="{ color: colorOf(e.color) }">
      {{ e.text }}
    </div>
    <div v-if="cpu.log.length === 0" class="empty">
      Cargue un programa y presione STEP para ver eventos.
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
