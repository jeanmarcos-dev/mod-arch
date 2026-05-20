<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import {
  Download,
  StepForward,
  Play,
  Pause,
  RotateCcw,
  Eraser,
  Settings,
  Pin,
  PinOff,
  Share2,
  Square,
  UserRound,
  LogIn,
  LogOut,
  GripHorizontal,
} from '@lucide/vue';
import { useCpuStore } from '../../stores/cpu';
import { useAuthStore } from '../../stores/auth';
import { useSimBarStore } from '../../stores/simBar';
import { useDraggable } from '../../composables/useDraggable';
import ProgramPicker from '../ProgramPicker.vue';
import ShareModal from '../ShareModal.vue';
import Term from '../glossary/Term.vue';

const cpu = useCpuStore();
const auth = useAuthStore();
const bar = useSimBarStore();
const router = useRouter();

const panelRef = ref<HTMLElement | null>(null);
const settingsOpen = ref(false);
const settingsRoot = ref<HTMLElement | null>(null);

const aliasInput = ref(cpu.sessionInfo.label);
watch(() => cpu.sessionInfo.label, (lab) => { aliasInput.value = lab; });

function commitAlias() {
  const v = aliasInput.value.trim().slice(0, 32);
  if (v !== cpu.sessionInfo.label) cpu.setAlias(v);
}

const modeLabel = computed(() => (cpu.mode === 3 ? 'USER' : 'KERNEL'));
const modeClass = computed(() => (cpu.mode === 3 ? 'sim-pill--user' : 'sim-pill--kernel'));
const loggedIn = computed(() => Boolean(cpu.sessionInfo.user_name));
const viewer = computed(() => cpu.isViewer());
const canShare = computed(() => (cpu.sessionInfo?.role ?? 'none') === 'owner');
const isShared = computed(() => Boolean(cpu.sessionInfo?.view_token));

function hex(v: number | undefined): string {
  if (typeof v !== 'number') return '—';
  return '0x' + (v >>> 0).toString(16).toUpperCase().padStart(8, '0');
}

function onLoad() {
  if (!cpu.selectedProgram) return;
  cpu.load(cpu.selectedProgram);
}

function onClear() {
  cpu.clearLog();
}

function onQuantum(e: Event) {
  const v = Number((e.target as HTMLInputElement).value);
  if (Number.isFinite(v)) cpu.setQuantum(v);
}

function toggleSettings() {
  settingsOpen.value = !settingsOpen.value;
}

function onDocClick(e: MouseEvent) {
  if (!settingsRoot.value) return;
  if (!settingsRoot.value.contains(e.target as Node)) settingsOpen.value = false;
}
function onEsc(e: KeyboardEvent) {
  if (e.key === 'Escape') settingsOpen.value = false;
}
onMounted(() => {
  document.addEventListener('click', onDocClick);
  document.addEventListener('keydown', onEsc);
});
onBeforeUnmount(() => {
  document.removeEventListener('click', onDocClick);
  document.removeEventListener('keydown', onEsc);
});

const shareOpen = ref(false);
function openShare() { shareOpen.value = true; }
async function goLogin() { await router.push('/login'); }
async function doLogout() { await auth.logout(); }

// Floating mode: the panel teleports to <body> with a fixed position. We
// drive coords via useDraggable so the small "handle" row above the bar
// doubles as a drag grip and the store persists the final position.
const drag = useDraggable({
  initial: { x: bar.x, y: bar.y },
  elementRef: panelRef,
  clampToViewport: true,
  onEnd: (pos) => { bar.setPosition(pos.x, pos.y); },
});

watch(() => bar.mode, async (m) => {
  if (m === 'floating') {
    await nextTick();
    drag.setPosition({ x: bar.x, y: bar.y });
  }
});

function dockHeaderDblClick() {
  bar.dock();
}
</script>

<template>
  <Teleport to="body" :disabled="bar.mode === 'docked'">
    <section
      ref="panelRef"
      :class="['sim-bar', `sim-bar--${bar.mode}`, { 'sim-bar--viewer': viewer, 'sim-bar--dragging': drag.isDragging.value }]"
      :style="bar.mode === 'floating' ? { left: drag.position.value.x + 'px', top: drag.position.value.y + 'px' } : undefined"
      role="region"
      aria-label="Controles de simulación"
    >
      <header
        v-if="bar.mode === 'floating'"
        class="sim-bar__handle"
        @pointerdown="drag.start($event)"
        @dblclick="dockHeaderDblClick()"
      >
        <GripHorizontal :size="14" :stroke-width="2" />
        <span class="sim-bar__title">SIMULACIÓN</span>
        <button
          type="button"
          class="icon-btn"
          title="Volver a anclar arriba"
          @click="bar.dock()"
        ><Pin :size="14" :stroke-width="2" /></button>
      </header>

      <div class="sim-bar__row sim-bar__row--controls">
        <div class="sim-bar__left">
          <span class="sim-bar__brand">MODOS-CPU</span>
        </div>

        <div class="sim-bar__center">
          <ProgramPicker class="sim-bar__picker" />

          <div class="sim-bar__btns" role="group" aria-label="Controles de ejecución">
            <button
              type="button"
              class="sim-btn"
              :disabled="!cpu.selectedProgram || viewer"
              title="Cargar el demo seleccionado"
              @click="onLoad"
            ><Download :size="14" :stroke-width="2" /><span>LOAD</span></button>
            <button
              type="button"
              class="sim-btn"
              :disabled="cpu.halted || cpu.running || viewer"
              title="Ejecutar una instrucción"
              @click="cpu.step()"
            ><StepForward :size="14" :stroke-width="2" /><span>STEP</span></button>
            <button
              v-if="!cpu.running"
              type="button"
              class="sim-btn"
              :disabled="cpu.halted || viewer"
              title="Ejecutar continuamente"
              @click="cpu.run(cpu.stepHz)"
            ><Play :size="14" :stroke-width="2" /><span>RUN</span></button>
            <button
              v-else
              type="button"
              class="sim-btn"
              :disabled="viewer"
              title="Pausar el modo continuo"
              @click="cpu.pause()"
            ><Pause :size="14" :stroke-width="2" /><span>PAUSE</span></button>
            <button
              type="button"
              class="sim-btn sim-btn--reset"
              :disabled="viewer"
              title="Resetear kernel, procesos y log"
              @click="cpu.reset()"
            ><RotateCcw :size="14" :stroke-width="2" /><span>RESET</span></button>
            <button
              type="button"
              class="sim-btn sim-btn--clear"
              :disabled="viewer"
              title="Limpiar el log de eventos y la consola, sin tocar el programa cargado"
              @click="onClear"
            ><Eraser :size="14" :stroke-width="2" /><span>CLEAR</span></button>
          </div>

          <div ref="settingsRoot" class="sim-bar__settings">
            <button
              type="button"
              class="icon-btn"
              :class="{ 'icon-btn--on': settingsOpen }"
              title="Velocidad y quantum"
              aria-label="Velocidad y quantum"
              @click="toggleSettings"
            ><Settings :size="16" :stroke-width="2" /></button>
            <div v-if="settingsOpen" class="sim-bar__popover" role="dialog" aria-label="Velocidad y quantum">
              <label class="sim-bar__field">
                <span class="sim-bar__field-label">HZ</span>
                <input
                  type="number"
                  min="1"
                  max="60"
                  v-model.number="cpu.stepHz"
                  :disabled="viewer"
                  aria-label="Pasos por segundo en modo RUN"
                />
                <span class="sim-bar__field-help">Pasos por segundo en RUN</span>
              </label>
              <label class="sim-bar__field">
                <Term term="Quantum" mode="label" class="sim-bar__field-label">QUANTUM</Term>
                <input
                  type="number"
                  min="0"
                  max="200"
                  :value="cpu.timerQuantum"
                  :disabled="viewer"
                  aria-label="Quantum del timer (ticks por proceso)"
                  @change="onQuantum"
                />
                <span class="sim-bar__field-help">Ticks antes de IRQ 0x20 (0 = apagado)</span>
              </label>
              <div class="sim-bar__field">
                <span class="sim-bar__field-label">CORES</span>
                <div class="sim-bar__core-picker" role="radiogroup" aria-label="Cantidad de núcleos">
                  <button
                    v-for="n in 4"
                    :key="n"
                    type="button"
                    role="radio"
                    :aria-checked="cpu.numCores === n"
                    :class="['sim-bar__core-pick', { 'sim-bar__core-pick--active': cpu.numCores === n }]"
                    :disabled="viewer"
                    :title="`Cambiar a ${n} núcleo${n === 1 ? '' : 's'} (reinicia el simulador)`"
                    @click="cpu.setNumCores(n)"
                  >{{ n }}</button>
                </div>
                <span class="sim-bar__field-help">Cambiarlo reinicia el simulador</span>
              </div>
            </div>
          </div>
        </div>

        <div class="sim-bar__right">
          <div
            v-if="loggedIn"
            class="sim-bar__user"
            :title="cpu.sessionInfo.user_email ? `Conectado como ${cpu.sessionInfo.user_name} · ${cpu.sessionInfo.user_email}` : `Conectado como ${cpu.sessionInfo.user_name}`"
          >
            <UserRound :size="14" :stroke-width="2" class="sim-bar__user-icon" />
            <span class="sim-bar__user-name">{{ cpu.sessionInfo.user_name }}</span>
            <button type="button" class="icon-btn icon-btn--ghost" title="Cerrar sesión" @click="doLogout">
              <LogOut :size="14" :stroke-width="2" />
            </button>
          </div>
          <label v-else-if="!viewer" class="sim-bar__alias" title="Identifíquese con un alias o inicie sesión con su correo.">
            <UserRound :size="14" :stroke-width="2" class="sim-bar__user-icon" />
            <input
              v-model="aliasInput"
              class="sim-bar__alias-input"
              type="text"
              name="alias"
              maxlength="32"
              placeholder="su alias…"
              autocomplete="off"
              aria-label="Alias visible para los espectadores"
              @blur="commitAlias"
              @keydown.enter="commitAlias"
            />
            <button type="button" class="icon-btn icon-btn--ghost" title="Iniciar sesión con correo" @click="goLogin">
              <LogIn :size="14" :stroke-width="2" />
            </button>
          </label>

          <button
            v-if="canShare"
            type="button"
            :class="['sim-bar__share', { 'sim-bar__share--on': isShared }]"
            :title="isShared ? 'Sesión compartida — clic para gestionarla' : 'Generar un enlace para que otros vean la sesión en modo lectura'"
            @click="openShare"
          >
            <component :is="isShared ? Square : Share2" :size="14" :stroke-width="2" />
            <span>{{ isShared ? 'Compartiendo' : 'Compartir' }}</span>
          </button>

          <button
            v-if="bar.mode === 'docked'"
            type="button"
            class="icon-btn"
            title="Desacoplar a ventana flotante"
            @click="bar.float()"
          ><PinOff :size="16" :stroke-width="2" /></button>

          <ShareModal :open="shareOpen" @close="shareOpen = false" />
        </div>
      </div>

      <div class="sim-bar__row sim-bar__row--status" aria-label="Estado del CPU">
        <span
          v-if="cpu.numCores > 1"
          class="sim-bar__core-tabs"
          role="tablist"
          aria-label="Núcleos disponibles"
          title="Elija qué núcleo se refleja en las píldoras inferiores"
        >
          <button
            v-for="c in cpu.cores"
            :key="c.id"
            type="button"
            role="tab"
            :aria-selected="c.id === cpu.selectedCoreId"
            :class="['sim-bar__core-tab', {
              'sim-bar__core-tab--active': c.id === cpu.selectedCoreId,
              'sim-bar__core-tab--idle': c.current_pid === 0,
              'sim-bar__core-tab--halted': c.halted,
            }]"
            :title="`Núcleo C${c.id}${c.current_pid ? ' · ejecutando P' + c.current_pid : ' · ocioso'}${c.halted ? ' · HALT' : ''}`"
            @click="cpu.setSelectedCore(c.id)"
          >C{{ c.id }}</button>
        </span>
        <span class="sim-pill">
          <Term term="PID" mode="label" class="sim-pill__k">PID</Term>
          <span class="sim-pill__v">{{ cpu.currentPID }}</span>
        </span>
        <span :class="['sim-pill', modeClass]">
          <Term :term="cpu.mode === 3 ? 'USER MODE (PL3)' : 'KERNEL MODE (PL0)'" mode="label" class="sim-pill__k">MODE</Term>
          <span class="sim-pill__v">{{ modeLabel }}</span>
        </span>
        <span class="sim-pill sim-pill--mono">
          <Term term="EIP" mode="label" class="sim-pill__k">EIP</Term>
          <span class="sim-pill__v">{{ hex(cpu.regs.EIP) }}</span>
        </span>
        <span class="sim-pill sim-pill--mono">
          <Term term="EAX" mode="label" class="sim-pill__k">EAX</Term>
          <span class="sim-pill__v">{{ hex(cpu.regs.EAX) }}</span>
        </span>
        <span class="sim-pill sim-pill--mono">
          <Term term="ESP" mode="label" class="sim-pill__k">ESP</Term>
          <span class="sim-pill__v">{{ hex(cpu.regs.ESP) }}</span>
        </span>
        <span class="sim-pill">
          <Term term="IRQ 0x20" mode="label" class="sim-pill__k">TICK</Term>
          <span class="sim-pill__v">{{ cpu.tick }}</span>
        </span>
        <span v-if="cpu.halted" class="sim-pill sim-pill--halt"><span class="sim-pill__v">HALTED</span></span>
        <span v-if="cpu.running" class="sim-pill sim-pill--running"><span class="sim-pill__v">RUNNING</span></span>
        <span :class="['sim-pill', cpu.connected ? 'sim-pill--ws-on' : 'sim-pill--ws-off']" :title="`WebSocket ${cpu.connected ? 'conectado' : 'desconectado'}`">
          <span class="sim-pill__k">WS</span><span class="sim-pill__v">{{ cpu.connected ? 'ON' : 'OFF' }}</span>
        </span>
      </div>
    </section>
  </Teleport>
</template>

<style scoped>
.sim-bar {
  background: var(--surface-1);
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
  color: var(--text);
}

.sim-bar--floating {
  position: fixed;
  z-index: var(--z-floating-toolbar, 250);
  background: var(--surface-1);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-elevation-floating, 0 8px 28px rgba(0, 0, 0, 0.55));
  max-width: calc(100vw - var(--space-4));
  overflow: visible;
}

.sim-bar--dragging { user-select: none; }

.sim-bar__handle {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-1) var(--space-3);
  border-bottom: 1px dashed var(--border);
  height: 28px;
  font-size: 9px;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--text-muted);
  cursor: grab;
}
.sim-bar--dragging .sim-bar__handle { cursor: grabbing; }

.sim-bar__title {
  font-family: 'Major Mono Display', monospace;
  letter-spacing: 3px;
  color: var(--amber);
  flex: 1;
}

.sim-bar__row {
  padding: var(--space-1) var(--space-3);
}

.sim-bar__row--controls {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
  align-items: center;
  gap: var(--space-2);
  min-height: 48px;
}

.sim-bar__left {
  justify-self: start;
  display: flex;
  align-items: center;
  gap: var(--space-2);
  min-width: 0;
}

.sim-bar__center {
  justify-self: center;
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.sim-bar__row--status {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
  min-height: 32px;
  border-top: 1px dashed var(--border);
  justify-content: flex-end;
}

.sim-bar__brand {
  font-family: 'Major Mono Display', monospace;
  letter-spacing: 3px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  font-size: 12px;
  white-space: nowrap;
}

.sim-bar__picker {
  flex: 0 1 200px;
  min-width: 140px;
}

.sim-bar__btns {
  display: flex;
  gap: var(--space-1);
  flex-wrap: nowrap;
}

.sim-btn {
  font-family: inherit;
  background: rgba(255, 179, 71, 0.05);
  border: 1px solid var(--border-strong);
  color: var(--amber);
  font-size: 11px;
  letter-spacing: 1px;
  text-transform: uppercase;
  padding: 0 10px;
  height: 32px;
  cursor: pointer;
  border-radius: var(--radius-sm);
  transition: background var(--transition-fast), box-shadow var(--transition-fast);
  white-space: nowrap;
  box-sizing: border-box;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.sim-btn > svg { flex-shrink: 0; }
.sim-btn:hover:not(:disabled) {
  background: rgba(255, 179, 71, 0.18);
  box-shadow: var(--shadow-glow-amber);
}
.sim-btn:disabled { opacity: 0.4; cursor: not-allowed; }
.sim-btn--reset {
  border-color: rgba(255, 58, 58, 0.35);
  color: var(--red-soft);
}
.sim-btn--reset:hover:not(:disabled) {
  background: rgba(255, 58, 58, 0.12);
  box-shadow: var(--shadow-glow-red);
}
.sim-btn--clear {
  border-color: var(--border);
  color: var(--text-dim);
}
.sim-btn--clear:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.04);
  color: var(--text);
}

.sim-bar__settings { position: relative; }

/* Unified icon-only buttons: every plain icon button across the bar uses
   this class so they all sit at the same 32x32 footprint and feel. */
.icon-btn {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  font-family: inherit;
  width: 32px;
  height: 32px;
  cursor: pointer;
  border-radius: var(--radius-sm);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  padding: 0;
  transition: background var(--transition-fast), color var(--transition-fast), border-color var(--transition-fast);
}
.icon-btn:hover, .icon-btn--on {
  background: rgba(255, 179, 71, 0.12);
  color: var(--amber);
  border-color: var(--border-strong);
}
.icon-btn--ghost {
  width: 22px;
  height: 22px;
  border: none;
}
.icon-btn--ghost:hover { background: rgba(255, 179, 71, 0.10); color: var(--amber); }
.icon-btn > svg { display: block; }

.sim-bar__popover {
  position: absolute;
  right: 0;
  top: calc(100% + var(--space-1));
  background: var(--surface-2);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-md);
  padding: var(--space-3);
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  min-width: 220px;
  z-index: var(--z-dropdown, 100);
  box-shadow: var(--shadow-elevation-2);
}

.sim-bar__field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 11px;
}
.sim-bar__field-label {
  font-size: 9px;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--text-muted);
}
.sim-bar__field input {
  font-family: 'JetBrains Mono', monospace;
  background: var(--surface-0);
  border: 1px solid var(--border);
  color: var(--text);
  padding: 4px 8px;
  border-radius: var(--radius-sm);
  font-size: 12px;
  width: 100%;
  box-sizing: border-box;
}
.sim-bar__field-help { color: var(--text-muted); font-size: 10px; }

.sim-bar__core-picker {
  display: inline-flex;
  gap: 4px;
}
.sim-bar__core-pick {
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  background: var(--surface-0);
  border: 1px solid var(--border);
  color: var(--text);
  width: 28px;
  height: 28px;
  cursor: pointer;
  border-radius: var(--radius-sm);
  transition: background var(--transition-fast), color var(--transition-fast), border-color var(--transition-fast);
}
.sim-bar__core-pick:hover:not(:disabled) {
  color: var(--amber);
  background: rgba(255, 179, 71, 0.10);
  border-color: var(--border-strong);
}
.sim-bar__core-pick--active {
  background: rgba(255, 179, 71, 0.18);
  color: var(--amber);
  border-color: var(--amber);
  box-shadow: var(--shadow-glow-amber);
}
.sim-bar__core-pick:disabled { opacity: 0.4; cursor: not-allowed; }

.sim-bar__core-tabs {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: 2px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: rgba(255, 179, 71, 0.03);
}
.sim-bar__core-tab {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  letter-spacing: 1px;
  text-transform: uppercase;
  background: transparent;
  border: 1px solid transparent;
  color: var(--text-muted);
  padding: 0 8px;
  height: 20px;
  cursor: pointer;
  border-radius: var(--radius-sm);
  display: inline-flex;
  align-items: center;
  transition: background var(--transition-fast), color var(--transition-fast), border-color var(--transition-fast);
}
.sim-bar__core-tab:hover {
  color: var(--amber);
  background: rgba(255, 179, 71, 0.08);
}
.sim-bar__core-tab--active {
  background: rgba(255, 179, 71, 0.18);
  color: var(--amber);
  border-color: var(--amber);
  box-shadow: var(--shadow-glow-amber);
}
.sim-bar__core-tab--idle:not(.sim-bar__core-tab--active) {
  color: var(--text-dim);
  opacity: 0.7;
}
.sim-bar__core-tab--halted:not(.sim-bar__core-tab--active) {
  color: var(--red-soft, #ff7a7a);
  opacity: 0.75;
}

.sim-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 0 6px;
  height: 24px;
  font-size: 10px;
  letter-spacing: 0.5px;
  background: rgba(255, 255, 255, 0.015);
  white-space: nowrap;
}
.sim-pill__k { color: var(--text-muted); letter-spacing: 1px; text-transform: uppercase; font-size: 9px; }
.sim-pill__v { color: var(--text); font-weight: 500; }
.sim-pill--mono .sim-pill__v { font-family: 'JetBrains Mono', monospace; font-size: 10px; }

.sim-pill--user { border-color: var(--green); }
.sim-pill--user .sim-pill__v { color: var(--green); }
.sim-pill--kernel { border-color: var(--red-soft, #ff7a7a); }
.sim-pill--kernel .sim-pill__v { color: var(--red-soft, #ff7a7a); }
.sim-pill--halt { border-color: var(--red); background: rgba(255, 58, 58, 0.06); }
.sim-pill--halt .sim-pill__v { color: var(--red); letter-spacing: 1.5px; font-weight: 700; }
.sim-pill--running { border-color: var(--green); }
.sim-pill--running .sim-pill__v { color: var(--green); }
.sim-pill--ws-on { border-color: var(--green); }
.sim-pill--ws-on .sim-pill__v { color: var(--green); }
.sim-pill--ws-off { border-color: var(--red); }
.sim-pill--ws-off .sim-pill__v { color: var(--red); }

.sim-bar__right {
  justify-self: end;
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.sim-bar__user, .sim-bar__alias {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: rgba(255, 179, 71, 0.06);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 0 4px 0 8px;
  height: 32px;
  box-sizing: border-box;
  color: var(--text);
}
.sim-bar__user-icon { color: var(--amber); }
.sim-bar__user-name { font-size: 11px; max-width: 140px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.sim-bar__alias-input {
  background: transparent;
  border: none;
  color: var(--text);
  font-family: inherit;
  font-size: 11px;
  width: 110px;
  outline: none;
}
.sim-bar__alias-input::placeholder { color: var(--text-muted); }

.sim-bar__share {
  font-family: inherit;
  background: rgba(255, 179, 71, 0.05);
  border: 1px solid var(--border);
  color: var(--text-dim);
  font-size: 10px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  height: 32px;
  padding: 0 12px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  box-sizing: border-box;
  cursor: pointer;
  border-radius: var(--radius-sm);
  transition: background var(--transition-fast), color var(--transition-fast), box-shadow var(--transition-fast);
  white-space: nowrap;
}
.sim-bar__share:hover { background: rgba(255, 179, 71, 0.12); color: var(--amber); border-color: var(--border-strong); }
.sim-bar__share--on {
  background: rgba(127, 255, 170, 0.10);
  color: var(--green);
  border-color: var(--green);
  box-shadow: var(--shadow-glow-green);
}

@media (max-width: 1100px) {
  .sim-bar__brand { display: none; }
}
@media (max-width: 900px) {
  .sim-bar__picker { display: none; }
  .sim-bar__user-name, .sim-bar__alias-input { display: none; }
}
</style>
