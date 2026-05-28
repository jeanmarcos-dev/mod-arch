<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useCpuStore, type PipeInfo } from '../stores/cpu';
import { usePreferencesStore } from '../stores/preferences';
import CollapsiblePanel from './ui/CollapsiblePanel.vue';

const cpu = useCpuStore();
const prefs = usePreferencesStore();
const { t } = useI18n();

const channels = computed(() => [...cpu.pipes].sort((a, b) => a.pipe_id - b.pipe_id));

// Render the pipe buffer as filled slots up to buf_level + empty placeholders
// up to buf_cap. Long buffers (>16 bytes) are summarised to keep the strip
// readable inside the narrow col-right column.
function bufferGlyphs(p: PipeInfo): { filled: string; empty: number } {
  const filled = p.last_bytes && p.last_op === 'write'
    ? bytesToAscii(p.last_bytes).slice(0, p.buf_level)
    : '·'.repeat(p.buf_level);
  return { filled, empty: Math.max(0, p.buf_cap - p.buf_level) };
}

function bytesToAscii(bytes: number[] | undefined): string {
  if (!bytes) return '';
  return bytes.map(b => (b >= 0x20 && b < 0x7F) ? String.fromCharCode(b) : '·').join('');
}

function headerFor(p: PipeInfo): string {
  if (p.is_fifo) return t('panels.ipc.fifoLabel', { name: p.name ?? `#${p.pipe_id}` });
  return t('panels.ipc.pipeLabel', { id: p.pipe_id });
}

function channelCountLabel(n: number): string {
  return n === 1 ? t('panels.ipc.channelsOne') : t('panels.ipc.channelsMany', { n });
}

function lastOpLabel(p: PipeInfo): string {
  return p.last_op === 'write'
    ? t('panels.ipc.lastWrite', { pid: p.last_pid })
    : t('panels.ipc.lastRead', { pid: p.last_pid });
}
</script>

<template>
  <CollapsiblePanel
    class="panel ipc"
    :title="t('panels.ipc.title')"
    :collapsed="prefs.panelsCollapsed.ipc"
    @update:collapsed="prefs.togglePanel('ipc', $event)"
  >
    <template #header-actions>
      <span class="ipc__count">{{ channelCountLabel(channels.length) }}</span>
    </template>

    <div v-if="channels.length === 0" class="ipc__empty">
      {{ t('panels.ipc.emptyBefore') }}<code>sys_pipe</code>{{ t('panels.ipc.emptyAfter') }}
    </div>

    <div v-else class="ipc__list">
      <article
        v-for="p in channels"
        :key="p.pipe_id"
        class="ipc-channel"
        :class="{ 'ipc-channel--fifo': p.is_fifo, 'ipc-channel--write': p.last_op === 'write', 'ipc-channel--read': p.last_op === 'read' }"
      >
        <header class="ipc-channel__head">
          <span class="ipc-channel__kind">{{ headerFor(p) }}</span>
          <span class="ipc-channel__owner">{{ t('panels.ipc.ownerCreatedBy', { pid: p.owner_pid }) }}</span>
        </header>
        <div class="ipc-channel__buffer" :title="t('panels.ipc.bufferTitle', { level: p.buf_level, cap: p.buf_cap })">
          <span class="ipc-channel__filled">{{ bufferGlyphs(p).filled || (p.buf_level > 0 ? '·'.repeat(p.buf_level) : '') }}</span>
          <span class="ipc-channel__empty">{{ '·'.repeat(bufferGlyphs(p).empty) }}</span>
          <span class="ipc-channel__meta">{{ p.buf_level }}/{{ p.buf_cap }} B</span>
        </div>
        <footer class="ipc-channel__foot">
          <span class="ipc-channel__fd" :title="t('panels.ipc.fdReadTitle')">fdR={{ p.fd_r }}</span>
          <span class="ipc-channel__fd" :title="t('panels.ipc.fdWriteTitle')">fdW={{ p.fd_w }}</span>
          <span v-if="p.last_op" class="ipc-channel__last">
            {{ lastOpLabel(p) }}
          </span>
        </footer>
      </article>
    </div>
  </CollapsiblePanel>
</template>

<style scoped>
.ipc {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  color: var(--text);
}

.ipc__count {
  color: var(--text-dim);
  font-size: 10px;
  letter-spacing: 1px;
}

.ipc__empty {
  color: var(--text-muted);
  font-size: 10px;
  line-height: 1.4;
  padding: 8px 0;
}

.ipc__empty code {
  color: var(--amber);
}

.ipc__list {
  display: flex;
  flex-direction: column;
  gap: 5px;
}

.ipc-channel {
  border: 1px solid var(--border);
  padding: 6px 8px;
  background: rgba(var(--accent-rgb), 0.04);
  display: flex;
  flex-direction: column;
  gap: 3px;
  transition: border-color 200ms ease, background 200ms ease;
}

.ipc-channel--write {
  border-color: var(--amber);
  background: rgba(var(--accent-rgb), 0.1);
}
.ipc-channel--read {
  border-color: var(--green);
  background: rgba(0, 255, 130, 0.06);
}
.ipc-channel--fifo {
  border-style: dashed;
}

.ipc-channel__head {
  display: flex;
  justify-content: space-between;
  font-size: 10px;
  letter-spacing: 1px;
}

.ipc-channel__kind {
  color: var(--amber);
  font-weight: 700;
}

.ipc-channel__owner {
  color: var(--text-dim);
}

.ipc-channel__buffer {
  font-family: 'JetBrains Mono', monospace;
  letter-spacing: 2px;
  font-size: 11px;
  display: flex;
  gap: 6px;
  align-items: baseline;
}

.ipc-channel__filled {
  color: var(--amber);
}

.ipc-channel__empty {
  color: var(--text-muted);
  opacity: 0.45;
}

.ipc-channel__meta {
  margin-left: auto;
  font-size: 9px;
  color: var(--text-dim);
  letter-spacing: 1px;
}

.ipc-channel__foot {
  display: flex;
  gap: 8px;
  font-size: 9px;
  color: var(--text-dim);
  letter-spacing: 1px;
}

.ipc-channel__last {
  margin-left: auto;
  color: var(--amber);
  font-weight: 600;
}
</style>
