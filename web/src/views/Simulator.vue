<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useCpuStore } from '../stores/cpu';
import TopBar from '../components/TopBar.vue';
import CpuPanel from '../components/CpuPanel.vue';
import EventsLog from '../components/EventsLog.vue';
import ProcessesPanel from '../components/ProcessesPanel.vue';
import ConsolePanel from '../components/ConsolePanel.vue';
import GlossaryModal from '../components/GlossaryModal.vue';
import MemoryPanel from '../components/MemoryPanel.vue';
import CodePanel from '../components/CodePanel.vue';

const cpu = useCpuStore();
const glossaryOpen = ref(false);

// En desktop hay 3 paneles siempre visibles (CPU izq, middle, Procs der) y el
// middleTab controla qué panel del centro se ve.
const middleTab = ref<'events' | 'memory' | 'code'>('events');

// En mobile (≤ 768px) solo se ve UN panel a la vez. mobileTab controla cuál.
// Si cambiás a code/events/memory en mobile, también actualizamos middleTab
// para que al rotar a desktop quede consistente.
type MobileTab = 'cpu' | 'code' | 'events' | 'memory' | 'procs' | 'console';
const mobileTab = ref<MobileTab>('events');

watch(mobileTab, (val) => {
  if (val === 'code' || val === 'events' || val === 'memory') {
    middleTab.value = val;
  }
});

watch(middleTab, (val) => {
  if (mobileTab.value === 'code' || mobileTab.value === 'events' || mobileTab.value === 'memory') {
    mobileTab.value = val;
  }
});

const isMobile = ref(false);
function checkMobile() {
  isMobile.value = window.matchMedia('(max-width: 768px)').matches;
}

// Visibilidad de cada panel resuelta en un solo lugar. En desktop, CPU/Procs/Console
// siempre visibles + middleTab para code/events/memory. En mobile, mobileTab decide todo.
const visible = computed(() => {
  if (isMobile.value) {
    const t = mobileTab.value;
    return {
      cpu: t === 'cpu',
      code: t === 'code',
      events: t === 'events',
      memory: t === 'memory',
      procs: t === 'procs',
      console: t === 'console',
      middle: t === 'code' || t === 'events' || t === 'memory' || t === 'console',
    };
  }
  return {
    cpu: true,
    code: middleTab.value === 'code',
    events: middleTab.value === 'events',
    memory: middleTab.value === 'memory',
    procs: true,
    console: true,
    middle: true,
  };
});

watch(() => cpu.glossaryOpenSignal, () => {
  glossaryOpen.value = true;
});

onMounted(() => {
  cpu.connect();
  checkMobile();
  window.addEventListener('resize', checkMobile);
});

onUnmounted(() => {
  window.removeEventListener('resize', checkMobile);
});
</script>

<template>
  <div :class="{ shake: cpu.faultShake }" class="root">
    <!-- Banner que sólo aparece para viewers (read-only). Reafirma a quién está mirando. -->
    <div v-if="cpu.isViewer()" class="viewer-banner" :title="`Sesión: ${cpu.sessionInfo.id}`">
      👁 Está viendo la sesión de <strong>{{ cpu.sessionInfo.label || ('Sesión ' + cpu.sessionInfo.id.slice(0, 6)) }}</strong>
      en modo <strong>solo lectura</strong> — los controles los maneja el propietario
    </div>

    <TopBar @open-glossary="cpu.openGlossary()" />

    <!-- Tab bar mobile-only — controla todo el layout. Sólo visible <= 768px (CSS) -->
    <nav class="mobile-tabs" :aria-hidden="!isMobile">
      <button :class="{ active: mobileTab === 'cpu' }" @click="mobileTab = 'cpu'" title="Registros del CPU">⚙ CPU</button>
      <button :class="{ active: mobileTab === 'code' }" @click="mobileTab = 'code'" title="Código C">⎇ CÓD</button>
      <button :class="{ active: mobileTab === 'events' }" @click="mobileTab = 'events'" title="Log de eventos">▤ LOG</button>
      <button :class="{ active: mobileTab === 'memory' }" @click="mobileTab = 'memory'" title="Memoria">▦ MEM</button>
      <button :class="{ active: mobileTab === 'procs' }" @click="mobileTab = 'procs'" title="Procesos y PCB">⎯ PROC</button>
      <button :class="{ active: mobileTab === 'console' }" @click="mobileTab = 'console'" title="Consola + teclado">▷ CONS</button>
    </nav>

    <main class="layout">
      <CpuPanel v-show="visible.cpu" class="col-left" />
      <div v-show="visible.middle" class="middle col-center">
        <div class="middle-tabs desktop-only">
          <button
            class="tab"
            :class="{ active: middleTab === 'code' }"
            @click="middleTab = 'code'"
            title="Código fuente C de los procesos cargados. El kernel los compiló (manualmente) a binarios x86 antes de cargarlos."
          >⎇ CÓDIGO C</button>
          <button
            class="tab"
            :class="{ active: middleTab === 'events' }"
            @click="middleTab = 'events'"
            title="Stream cronológico de eventos del simulador: instrucciones, mode switches, syscalls, etc."
          >▤ EVENT LOG</button>
          <button
            class="tab"
            :class="{ active: middleTab === 'memory' }"
            @click="middleTab = 'memory'"
            title="Vista hex dump de la memoria física. Puede saltar al EIP, al stack, o a cualquier dirección."
          >▦ MEMORIA</button>
        </div>
        <CodePanel v-show="visible.code" />
        <EventsLog v-show="visible.events" />
        <MemoryPanel v-show="visible.memory" />
        <ConsolePanel v-show="visible.console" />
      </div>
      <ProcessesPanel v-show="visible.procs" class="col-right" />
    </main>

    <GlossaryModal :open="glossaryOpen" :initial-search="cpu.glossarySearch" @close="glossaryOpen = false" />

    <div class="credit" title="Demo de Sistemas Operativos">
      <span class="credit-tag">DEMO_SO · UCV</span>
      <span class="credit-name">jeanmarcos Juárez</span>
    </div>
  </div>
</template>

<style scoped>
.layout {
  display: flex;
  flex: 1;
  gap: 8px;
  padding: 8px;
  overflow: hidden;
}

.middle {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 0;
}

.middle-tabs {
  display: flex;
  gap: 4px;
}

.tab {
  flex: 1;
  font-size: 12px;
  letter-spacing: 1px;
  padding: 6px 10px;
  color: var(--text-dim);
  border-color: var(--border);
  background: rgba(0, 0, 0, 0.3);
}

.tab.active {
  color: var(--amber);
  border-color: var(--amber);
  background: rgba(255, 179, 71, 0.1);
  box-shadow: 0 0 8px rgba(255, 179, 71, 0.2);
}

.credit {
  position: fixed;
  bottom: 6px;
  right: 10px;
  z-index: 100;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 1px;
  pointer-events: none;
  user-select: none;
  font-family: 'JetBrains Mono', monospace;
  line-height: 1.1;
  opacity: 0.65;
  transition: opacity 0.2s ease;
}

.credit:hover {
  opacity: 1;
}

.credit-tag {
  font-family: 'Major Mono Display', monospace;
  font-size: 11px;
  color: var(--amber);
  letter-spacing: 3px;
  text-shadow: 0 0 6px rgba(255, 179, 71, 0.3);
}

.credit-name {
  font-size: 10px;
  color: var(--text-dim);
  letter-spacing: 1px;
  text-transform: uppercase;
}

.root {
  display: flex;
  flex-direction: column;
  height: 100%;
  flex: 1;
}

@keyframes shake-anim {
  0%, 100% { transform: translateX(0); }
  10%, 30%, 50%, 70%, 90% { transform: translateX(-6px); }
  20%, 40%, 60%, 80% { transform: translateX(6px); }
}

.shake {
  animation: shake-anim 500ms cubic-bezier(.36,.07,.19,.97) both;
}

/* Banner viewer — fija pero suave, no compite con el TopBar. */
.viewer-banner {
  background: rgba(255, 213, 79, 0.12);
  border-bottom: 1px solid var(--yellow);
  color: var(--yellow);
  padding: 8px 14px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  text-align: center;
  letter-spacing: 0.5px;
}

.viewer-banner strong {
  color: var(--amber);
  letter-spacing: 1px;
}

/* Mobile tab bar — oculta en desktop, visible vía media query mobile */
.mobile-tabs {
  display: none;
}

/* ─── MOBILE (≤ 768px) ─────────────────────────────────────────────────────
   En mobile sólo se muestra un panel a la vez, controlado por mobileTab.
   El layout pasa a columna vertical y la mobile-tabs bar es la única forma
   de navegar entre paneles. */
@media (max-width: 768px) {
  .layout {
    flex-direction: column;
    padding: 6px;
    gap: 6px;
  }

  .mobile-tabs {
    display: grid;
    grid-template-columns: repeat(6, 1fr);
    gap: 2px;
    padding: 6px;
    background: rgba(0, 0, 0, 0.4);
    border-bottom: 1px solid var(--border-strong);
    position: sticky;
    top: 0;
    z-index: 50;
  }

  .mobile-tabs button {
    font-size: 10px;
    padding: 8px 2px;
    letter-spacing: 0.5px;
    color: var(--text-dim);
    border: 1px solid var(--border);
    background: rgba(0, 0, 0, 0.5);
    min-height: 42px;
    text-transform: uppercase;
  }

  .mobile-tabs button.active {
    color: var(--amber);
    border-color: var(--amber);
    background: rgba(255, 179, 71, 0.15);
    box-shadow: 0 0 6px rgba(255, 179, 71, 0.3);
  }

  /* Las tabs internas del middle desaparecen — su función la cubre la mobile-tabs bar. */
  .desktop-only {
    display: none !important;
  }

  /* Credit más discreto en mobile, no estorba */
  .credit {
    bottom: 4px;
    right: 6px;
    opacity: 0.35;
  }

  .credit-tag { font-size: 9px; letter-spacing: 2px; }
  .credit-name { font-size: 8px; }
}
</style>
