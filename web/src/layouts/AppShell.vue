<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import {
  LayoutDashboard,
  ListOrdered,
  MemoryStick,
  Zap,
  Layers,
  Cpu,
  FolderClock,
  BookOpen,
  Settings,
  UserRound,
} from '@lucide/vue';
import { useCpuStore } from '../stores/cpu';
import { usePreferencesStore } from '../stores/preferences';
import Sidebar from '../components/shell/Sidebar.vue';
import Topbar from '../components/shell/Topbar.vue';
import SimBar from '../components/shell/SimBar.vue';
import GlossaryModal from '../components/GlossaryModal.vue';

const cpu = useCpuStore();
const prefs = usePreferencesStore();
const route = useRoute();

// Any cpu.openGlossary('EIP') call from a panel opens the popup right where
// the student clicked. The full /glosario page is still reachable from the
// sidebar for browse-the-whole-thing scenarios.
const glossaryOpen = ref(false);
watch(() => cpu.glossaryOpenSignal, (n, prev) => {
  if (n === prev) return;
  glossaryOpen.value = true;
});

// Connect once when the shell mounts. The shell lives for the whole student
// session, so child route changes don't trigger a reconnect. cpu.connect is
// idempotent — calls from Simulator.vue (for the viewer route) are no-ops
// here.
onMounted(() => {
  cpu.connect();
});

// The breadcrumb was removed from the topbar, so the section name now lives
// only in document.title (and in the active sidebar item). This avoids a
// redundant chrome row.
watch(
  () => route.meta.title,
  (title) => {
    const t = typeof title === 'string' && title ? `${title} · modos-cpu` : 'modos-cpu';
    if (typeof document !== 'undefined') document.title = t;
  },
  { immediate: true },
);

const items = [
  { to: '/', label: 'Vista general', icon: LayoutDashboard, group: 'SIMULACIÓN' },
  { to: '/procesos', label: 'Procesos', icon: ListOrdered, group: 'SIMULACIÓN' },
  { to: '/memoria', label: 'Memoria', icon: MemoryStick, group: 'SIMULACIÓN' },
  { to: '/interrupciones', label: 'Interrupciones', icon: Zap, group: 'SIMULACIÓN' },
  { to: '/concurrencia', label: 'Concurrencia', icon: Layers, group: 'SIMULACIÓN' },
  { to: '/x86', label: 'x86 real', icon: Cpu, group: 'SIMULACIÓN', soon: true },
  { to: '/sesiones', label: 'Mis sesiones', icon: FolderClock, group: 'HERRAMIENTAS' },
  { to: '/glosario', label: 'Glosario', icon: BookOpen, group: 'HERRAMIENTAS' },
  { to: '/preferencias', label: 'Preferencias', icon: Settings, group: 'HERRAMIENTAS' },
  { to: '/perfil', label: 'Perfil', icon: UserRound, group: 'HERRAMIENTAS' },
];

// Routes where the SimBar (controls + status chips) is meaningful. Tool
// routes like /perfil, /preferencias, /glosario, /sesiones don't drive the
// simulator, so the bar is hidden to give them full vertical space.
const isSimRoute = computed(() => {
  const p = route.path;
  return p === '/'
    || p === '/procesos'
    || p === '/memoria'
    || p === '/interrupciones'
    || p === '/concurrencia'
    || p === '/x86';
});
</script>

<template>
  <div :class="['shell', `shell--mode-${prefs.mode}`]">
    <Sidebar :items="items" />
    <div class="shell__main">
      <SimBar v-if="isSimRoute" />
      <Topbar v-else />
      <main class="shell__content">
        <RouterView v-slot="{ Component }">
          <Transition name="route" mode="out-in">
            <component :is="Component" />
          </Transition>
        </RouterView>
      </main>
    </div>

    <GlossaryModal
      :open="glossaryOpen"
      :initial-search="cpu.glossarySearch"
      @close="glossaryOpen = false"
    />
  </div>
</template>

<style scoped>
.shell {
  display: flex;
  height: 100vh;
  width: 100vw;
  overflow: hidden;
}

.shell__main {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}

.shell__content {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

/* Mode-driven density and font-size knobs. Sections opt in by referencing the
   --row-density variable and font-size: inherit. */
.shell--mode-pedagogic { --row-density: 1.15; font-size: 16px; }
.shell--mode-projector { --row-density: 1.4; font-size: 18px; }

/* Route transition: gentle slide+fade so changing sections feels light but
   noticeable. Honors prefers-reduced-motion via the global rule in style.css. */
.route-enter-from { opacity: 0; transform: translateY(8px); }
.route-leave-to   { opacity: 0; transform: translateY(-6px); }
.route-enter-active,
.route-leave-active { transition: opacity var(--transition-base), transform var(--transition-base); }
</style>
