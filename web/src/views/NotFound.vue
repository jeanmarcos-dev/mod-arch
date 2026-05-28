<script setup lang="ts">
import { useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { ArrowLeft, LogIn } from '@lucide/vue';

const { t } = useI18n();
const route = useRoute();
</script>

<template>
  <div class="not-found">
    <div class="card">
      <h1>404</h1>
      <p class="path">{{ route.fullPath }}</p>
      <p class="hint">{{ t('notFound.hint') }}</p>
      <div class="links">
        <a href="/"><ArrowLeft :size="12" :stroke-width="2" /><span>{{ t('notFound.goSim') }}</span></a>
        <a href="/login"><LogIn :size="12" :stroke-width="2" /><span>{{ t('notFound.goLogin') }}</span></a>
      </div>
    </div>
  </div>
</template>

<style scoped>
.not-found {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 24px;
  background:
    radial-gradient(ellipse at center, rgba(var(--accent-rgb), 0.05), transparent 60%),
    var(--bg, #0a0a0a);
}

.card {
  width: min(100%, 480px);
  text-align: center;
  border: 1px solid var(--border, #3a3a3a);
  background: var(--surface-overlay-strong);
  padding: 36px 28px;
  font-family: 'JetBrains Mono', monospace;
}

h1 {
  font-family: var(--font-display);
  font-size: 64px;
  color: var(--amber, #ffb347);
  letter-spacing: 12px;
  margin: 0 0 16px;
  text-shadow: 0 0 14px rgba(var(--accent-rgb), 0.35);
}

@media (max-width: 480px) {
  h1 { font-size: 44px; letter-spacing: 8px; }
  .card { padding: 24px 18px; }
}

.path {
  font-size: 13px;
  color: var(--text, #e0e0e0);
  background: rgba(var(--accent-rgb), 0.06);
  border: 1px solid rgba(var(--accent-rgb), 0.15);
  padding: 6px 10px;
  word-break: break-all;
  margin: 0 0 14px;
}

.hint {
  color: var(--text-dim, #888);
  font-size: 12px;
  margin: 0 0 22px;
  line-height: 1.5;
}

.links {
  display: flex;
  justify-content: center;
  gap: 18px;
  font-size: 11px;
  letter-spacing: 1px;
}

.links a {
  color: var(--text-dim, #888);
  text-decoration: none;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.links a:hover {
  color: var(--amber, #ffb347);
}
</style>
