<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { useCpuStore } from '../stores/cpu';
import { usePreferencesStore } from '../stores/preferences';
import CollapsiblePanel from './ui/CollapsiblePanel.vue';

const cpu = useCpuStore();
const prefs = usePreferencesStore();
const { t } = useI18n();
</script>

<template>
  <CollapsiblePanel
    class="panel queues"
    :title="t('panels.queues.title')"
    :title-hint="t('panels.queues.titleHint')"
    glossary-key="Ready queue"
    :collapsed="prefs.panelsCollapsed.queues"
    @update:collapsed="prefs.togglePanel('queues', $event)"
  >
    <div class="ready-queue">
      <div
        class="rq-label clickable-label"
        :title="t('panels.processes.readyQueueHint')"
        @click="cpu.openGlossary('Ready queue')"
      >{{ t('panels.processes.readyQueue') }}</div>
      <div class="rq-pills">
        <span v-if="cpu.readyQueue.length === 0" class="rq-empty">{{ t('panels.processes.empty') }}</span>
        <span v-for="pid in cpu.readyQueue" :key="pid" class="rq-pill">P{{ pid }}</span>
      </div>
    </div>

    <div class="wait-queue">
      <div
        class="rq-label clickable-label"
        :title="t('panels.processes.waitQueueHint')"
        @click="cpu.openGlossary('Waiting queue')"
      >{{ t('panels.processes.waitQueue') }}
        <span v-if="cpu.waitQueue.length > 0" class="wq-hint">{{ t('panels.processes.waitQueueHelp') }}</span>
      </div>
      <div class="rq-pills">
        <span v-if="cpu.waitQueue.length === 0" class="rq-empty">{{ t('panels.processes.empty') }}</span>
        <span
          v-for="w in cpu.waitQueue"
          :key="w.pid"
          class="wq-pill"
          :title="t('panels.processes.waitTooltip', { pid: w.pid, reason: w.reason })"
        >
          P{{ w.pid }}
          <span class="wq-reason">{{ w.reason }}</span>
        </span>
      </div>
    </div>
  </CollapsiblePanel>
</template>

<style scoped>
.queues {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.clickable-label {
  cursor: pointer;
  transition: color 0.15s ease;
}

.clickable-label:hover {
  color: var(--cyan);
  text-shadow: 0 0 6px rgba(106, 212, 255, 0.4);
}

.ready-queue {
  padding-bottom: 10px;
}

.wait-queue {
  margin-top: 10px;
  padding-top: 8px;
  border-top: 1px dashed var(--border);
}

.rq-label {
  color: var(--cyan);
  font-size: 11px;
  letter-spacing: 1px;
  text-transform: uppercase;
  margin-bottom: 6px;
}

.rq-pills {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

.rq-pill {
  padding: 2px 8px;
  background: rgba(255, 213, 79, 0.08);
  border: 1px solid var(--yellow);
  color: var(--yellow);
  font-size: 13px;
  font-weight: 700;
}

.rq-empty {
  color: var(--text-muted);
  font-style: italic;
  font-size: 13px;
}

.wq-hint {
  color: var(--text-muted);
  font-style: italic;
  font-size: 10px;
  text-transform: none;
  letter-spacing: 0;
  margin-left: 4px;
}

.wq-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 8px;
  background: rgba(255, 122, 122, 0.06);
  border: 1px solid var(--red-soft);
  color: var(--red-soft);
  font-size: 13px;
  font-weight: 700;
  animation: wait-pulse 1.6s ease-in-out infinite;
}

.wq-reason {
  font-size: 11px;
  font-weight: 400;
  color: var(--text-dim);
  text-transform: lowercase;
  letter-spacing: 0;
}

@keyframes wait-pulse {
  0%, 100% { opacity: 1; box-shadow: none; }
  50%      { opacity: 0.7; box-shadow: 0 0 6px rgba(255, 122, 122, 0.3); }
}
</style>
