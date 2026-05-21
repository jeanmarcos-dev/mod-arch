<script setup lang="ts">
import { onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { useCpuStore } from '../stores/cpu';

// ViewerEntry handles the /v/:token landing. The backend has already set the
// mc_view cookie on the GET that served index.html, so by the time this
// component mounts the SPA is identified as a viewer. We just open the WS,
// wait for the initial session_info, then jump to the section the host is
// currently presenting (or / if none has been broadcast yet).
const router = useRouter();
const cpu = useCpuStore();

onMounted(async () => {
  cpu.connect();
  // Decide the landing path: presence-driven if available, '/' otherwise.
  // Give the WS up to ~1.5s to deliver the initial session_info before
  // falling back, so a freshly opened viewer link still lands on the right
  // section without an extra flash of "/".
  const deadline = Date.now() + 1500;
  while (!cpu.lastPresence.section && Date.now() < deadline) {
    await new Promise((r) => window.setTimeout(r, 50));
  }
  const target = cpu.lastPresence.section || '/';
  await router.replace(target).catch(() => undefined);
});
</script>

<template>
  <div class="viewer-entry">
    <div class="viewer-entry__inner">
      <span class="viewer-entry__brand">MODOS-CPU</span>
      <span class="viewer-entry__msg">Conectando a la sesión del anfitrión…</span>
    </div>
  </div>
</template>

<style scoped>
.viewer-entry {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100vh;
  width: 100vw;
  background: var(--surface-0);
}
.viewer-entry__inner {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  align-items: center;
  color: var(--text-dim);
  font-family: 'JetBrains Mono', monospace;
}
.viewer-entry__brand {
  font-family: 'Major Mono Display', monospace;
  letter-spacing: 4px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  font-size: 18px;
}
.viewer-entry__msg { font-size: 12px; letter-spacing: 1px; }
</style>
