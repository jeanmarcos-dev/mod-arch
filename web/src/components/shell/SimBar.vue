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
  Crosshair,
} from '@lucide/vue';
import { useCpuStore } from '../../stores/cpu';
import { useAuthStore } from '../../stores/auth';
import { useSimBarStore } from '../../stores/simBar';
import { useWhiteboardStore } from '../../stores/whiteboard';
import { useDraggable } from '../../composables/useDraggable';
import ProgramPicker from '../ProgramPicker.vue';
import ShareModal from '../ShareModal.vue';
import Term from '../glossary/Term.vue';
import SimStatusRow from './SimStatusRow.vue';
import HandRaiseBadge from './HandRaiseBadge.vue';

const cpu = useCpuStore();
const auth = useAuthStore();
const bar = useSimBarStore();
const whiteboard = useWhiteboardStore();
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

const loggedIn = computed(() => Boolean(cpu.sessionInfo.user_name));
const viewer = computed(() => cpu.isViewer());
const canShare = computed(() => (cpu.sessionInfo?.role ?? 'none') === 'owner');
const isShared = computed(() => Boolean(cpu.sessionInfo?.view_token));
const whiteboardOn = computed(() => whiteboard.broadcasting);

// Toggling the whiteboard is a no-op for non-owners (button is hidden in
// that case anyway). When turning OFF we also send pointer_off so viewers
// drop the cursor immediately instead of waiting for the staleness timer.
function toggleWhiteboard() {
  const next = !whiteboard.broadcasting;
  whiteboard.setBroadcasting(next);
  if (!next) cpu.sendPointerOff();
}

function onLoad() {
  if (!cpu.selectedProgram) return;
  cpu.load(cpu.selectedProgram);
}

function onClear() {
  cpu.clearLog();
}

// Both HZ and QUANTUM apply on every keystroke through a small debounce so the
// backend gets one command per "settled" value instead of one per digit. The
// 200ms window also feels instant during a class while protecting the WS from
// a flurry of writes when the student scrubs the spinner.
const SETTING_DEBOUNCE_MS = 200;
let hzTimer: number | null = null;
let qTimer: number | null = null;

function onHz(e: Event) {
  const v = Number((e.target as HTMLInputElement).value);
  if (!Number.isFinite(v)) return;
  const clamped = Math.max(1, Math.min(60, Math.round(v)));
  if (hzTimer !== null) window.clearTimeout(hzTimer);
  hzTimer = window.setTimeout(() => {
    hzTimer = null;
    cpu.setSpeed(clamped);
  }, SETTING_DEBOUNCE_MS);
}

function onQuantum(e: Event) {
  const v = Number((e.target as HTMLInputElement).value);
  if (!Number.isFinite(v)) return;
  const clamped = Math.max(0, Math.min(200, Math.round(v)));
  if (qTimer !== null) window.clearTimeout(qTimer);
  qTimer = window.setTimeout(() => {
    qTimer = null;
    cpu.setQuantum(clamped);
  }, SETTING_DEBOUNCE_MS);
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
        <GripHorizontal :size="14" :stroke-width="2" class="sim-bar__handle-grip" />
        <span class="sim-bar__title">SIMULACIÓN</span>
        <button
          type="button"
          class="sim-bar__dock-btn"
          title="Volver a anclar arriba (doble click sobre la cabecera)"
          @click="bar.dock()"
        >
          <Pin :size="13" :stroke-width="2" />
          <span>Anclar</span>
        </button>
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
                  :value="cpu.stepHz"
                  :disabled="viewer"
                  aria-label="Pasos por segundo en modo RUN"
                  @input="onHz"
                />
                <span class="sim-bar__field-help">Pasos por segundo en RUN (se aplica en caliente)</span>
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
                  @input="onQuantum"
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

          <HandRaiseBadge v-if="canShare && isShared" />

          <button
            v-if="canShare"
            type="button"
            :class="['sim-bar__share sim-bar__whiteboard', { 'sim-bar__whiteboard--on': whiteboardOn }]"
            :title="whiteboardOn ? 'Pizarra activa — su puntero se muestra a los espectadores en la misma vista' : 'Comparta su puntero como pizarra para señalar en vivo'"
            @click="toggleWhiteboard"
          >
            <Crosshair :size="14" :stroke-width="2" />
            <span>{{ whiteboardOn ? 'Pizarra' : 'Pizarra' }}</span>
          </button>

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

      <SimStatusRow class="sim-bar__row sim-bar__row--status" />
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
  padding: var(--space-1) var(--space-2) var(--space-1) var(--space-3);
  border-bottom: 1px dashed var(--border);
  border-top-left-radius: var(--radius-md);
  border-top-right-radius: var(--radius-md);
  background: rgba(var(--accent-rgb), 0.04);
  min-height: 36px;
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
  font-size: 10px;
}

.sim-bar__handle-grip { color: var(--text-muted); flex-shrink: 0; }

.sim-bar__dock-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 26px;
  padding: 0 10px;
  font-family: inherit;
  font-size: 10px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  background: rgba(var(--accent-rgb), 0.06);
  border: 1px solid var(--border);
  color: var(--amber);
  border-radius: var(--radius-sm);
  cursor: pointer;
  flex-shrink: 0;
  transition: background var(--transition-fast), border-color var(--transition-fast), box-shadow var(--transition-fast);
}
.sim-bar__dock-btn:hover {
  background: rgba(var(--accent-rgb), 0.16);
  border-color: var(--amber);
  box-shadow: var(--shadow-glow-amber);
}
.sim-bar__dock-btn > svg { flex-shrink: 0; }

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
  border-top: 1px dashed var(--border);
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
  background: rgba(var(--accent-rgb), 0.05);
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
  background: rgba(var(--accent-rgb), 0.18);
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
  background: rgba(var(--accent-rgb), 0.12);
  color: var(--amber);
  border-color: var(--border-strong);
}
.icon-btn--ghost {
  width: 22px;
  height: 22px;
  border: none;
}
.icon-btn--ghost:hover { background: rgba(var(--accent-rgb), 0.10); color: var(--amber); }
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
  padding: 0;
  line-height: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  border-radius: var(--radius-sm);
  box-sizing: border-box;
  transition: background var(--transition-fast), color var(--transition-fast), border-color var(--transition-fast);
}
.sim-bar__core-pick:hover:not(:disabled) {
  color: var(--amber);
  background: rgba(var(--accent-rgb), 0.10);
  border-color: var(--border-strong);
}
.sim-bar__core-pick--active {
  background: rgba(var(--accent-rgb), 0.18);
  color: var(--amber);
  border-color: var(--amber);
  box-shadow: var(--shadow-glow-amber);
}
.sim-bar__core-pick:disabled { opacity: 0.4; cursor: not-allowed; }

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
  background: rgba(var(--accent-rgb), 0.06);
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
  background: rgba(var(--accent-rgb), 0.05);
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
.sim-bar__share:hover { background: rgba(var(--accent-rgb), 0.12); color: var(--amber); border-color: var(--border-strong); }

/* Whiteboard reuses the share-button geometry but flips to a green "active"
   palette when the owner is broadcasting their pointer, mirroring the
   "compartiendo" affordance for sharing. */
.sim-bar__whiteboard--on {
  background: rgba(127, 255, 170, 0.10);
  color: var(--green);
  border-color: var(--green);
  box-shadow: var(--shadow-glow-green);
}

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
