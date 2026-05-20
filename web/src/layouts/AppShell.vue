<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useAuthStore } from '../stores/auth';
import { useCpuStore } from '../stores/cpu';
import { usePreferencesStore } from '../stores/preferences';
import Sidebar from '../components/shell/Sidebar.vue';
import Topbar from '../components/shell/Topbar.vue';
import SimulationMinibar from '../components/shell/SimulationMinibar.vue';
import GlossaryModal from '../components/GlossaryModal.vue';

const auth = useAuthStore();
const cpu = useCpuStore();
const prefs = usePreferencesStore();
const route = useRoute();
const router = useRouter();

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

const items = [
  { to: '/', label: 'Vista general', icon: '▶', group: 'SIMULACIÓN' },
  { to: '/procesos', label: 'Procesos', icon: '●', group: 'SIMULACIÓN' },
  { to: '/memoria', label: 'Memoria', icon: '▤', group: 'SIMULACIÓN' },
  { to: '/interrupciones', label: 'Interrupciones', icon: '◈', group: 'SIMULACIÓN' },
  { to: '/concurrencia', label: 'Concurrencia', icon: '◫', group: 'SIMULACIÓN', soon: true },
  { to: '/x86', label: 'x86 real', icon: '▣', group: 'SIMULACIÓN', soon: true },
  { to: '/sesiones', label: 'Mis sesiones', icon: '◇', group: 'HERRAMIENTAS' },
  { to: '/glosario', label: 'Glosario', icon: '?', group: 'HERRAMIENTAS' },
  { to: '/preferencias', label: 'Preferencias', icon: '⚙', group: 'HERRAMIENTAS' },
  { to: '/perfil', label: 'Perfil', icon: '◐', group: 'HERRAMIENTAS' },
];

const showMinibar = computed(() => route.path !== '/' && route.meta.section === true);

async function logout() {
  await auth.logout();
  router.replace('/login');
}
</script>

<template>
  <div :class="['shell', `shell--mode-${prefs.mode}`]">
    <Sidebar :items="items" />
    <div class="shell__main">
      <Topbar>
        <template #actions>
          <button
            v-if="auth.user"
            type="button"
            class="shell__logout"
            title="Cerrar sesión"
            @click="logout"
          >
            ↩ Salir
          </button>
          <RouterLink v-else to="/login" class="shell__login-link">⤴ Identificarse</RouterLink>
        </template>
      </Topbar>
      <SimulationMinibar v-if="showMinibar" />
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

.shell__logout,
.shell__login-link {
  font-family: inherit;
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  font-size: 10px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  height: 32px;
  padding: 0 12px;
  display: inline-flex;
  align-items: center;
  box-sizing: border-box;
  cursor: pointer;
  text-decoration: none;
  border-radius: var(--radius-sm);
  transition: background var(--transition-fast), color var(--transition-fast);
}

.shell__logout:hover,
.shell__login-link:hover {
  background: rgba(255, 179, 71, 0.08);
  color: var(--amber);
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
