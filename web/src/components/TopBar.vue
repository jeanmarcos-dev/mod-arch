<script setup lang="ts">
import { useCpuStore } from '../stores/cpu';
import { useAuthStore } from '../stores/auth';
import { computed, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import ProgramPicker from './ProgramPicker.vue';

const cpu = useCpuStore();
const auth = useAuthStore();
const router = useRouter();

const modeLabel = computed(() => (cpu.mode === 3 ? 'USER' : 'KERNEL'));
const modeClass = computed(() => (cpu.mode === 3 ? 'mode-user' : 'mode-kernel'));

// The alias input mirrors sessionInfo.label and only shows for anonymous
// owners; once a user is logged in, the read-only name pill replaces it.
const aliasInput = ref(cpu.sessionInfo.label);
watch(() => cpu.sessionInfo.label, (lab) => { aliasInput.value = lab; });
function onAliasCommit() {
  const v = aliasInput.value.trim().slice(0, 32);
  if (v !== cpu.sessionInfo.label) cpu.setAlias(v);
}

const loggedIn = computed(() => Boolean(cpu.sessionInfo.user_name));

function goToLogin() {
  router.push('/login');
}

async function doLogout() {
  await auth.logout();
}

function onLoad() {
  if (cpu.selectedProgram) cpu.load(cpu.selectedProgram);
}

defineEmits<{ (e: 'open-glossary'): void; (e: 'open-share'): void }>();
</script>

<template>
  <div class="topbar">
    <div class="topbar-row">
      <div
        v-if="!cpu.isViewer() && loggedIn"
        class="user-wrap"
        :title="cpu.sessionInfo.user_email ? `Conectado como ${cpu.sessionInfo.user_name} · ${cpu.sessionInfo.user_email}` : `Conectado como ${cpu.sessionInfo.user_name}`"
      >
        <span class="user-icon">👤</span>
        <span class="user-name">{{ cpu.sessionInfo.user_name }}</span>
        <button class="btn-logout" @click="doLogout" title="Cerrar sesión y continuar de forma anónima">×</button>
      </div>

      <label
        v-else-if="!cpu.isViewer()"
        class="alias-wrap"
        title="Identifíquese con un alias breve. Si comparte su sesión, este nombre aparece en el banner del espectador y en la lista pública. También puede iniciar sesión con su correo para acceder desde otro dispositivo."
      >
        <span class="alias-icon">◉</span>
        <input
          v-model="aliasInput"
          @blur="onAliasCommit"
          @keydown.enter="onAliasCommit"
          class="alias-input"
          type="text"
          name="alias"
          maxlength="32"
          placeholder="su alias..."
          autocomplete="off"
          aria-label="Alias visible para los espectadores"
        />
        <button class="btn-login-mini" @click.prevent="goToLogin" title="Iniciar sesión con nombre y correo (opcional)">⤴</button>
      </label>

      <ProgramPicker />

      <div class="actions">
        <button @click="onLoad" :disabled="!cpu.selectedProgram || cpu.isViewer()" title="Carga el demo seleccionado en el simulador. Reinicia el kernel y arranca los procesos del demo.">▸ LOAD</button>
        <button @click="cpu.step" :disabled="cpu.halted || cpu.running || cpu.isViewer()" title="Ejecuta UNA instrucción (o el dispatch de la IRQ si hay alguna pendiente). Avanza Tick en 1.">⏵ STEP</button>
        <button v-if="!cpu.running" @click="cpu.run(cpu.stepHz)" :disabled="cpu.halted || cpu.isViewer()" title="Ejecuta steps automáticamente a Hz pasos por segundo.">⏩ RUN</button>
        <button v-else @click="cpu.pause" :disabled="cpu.isViewer()" title="Detiene el RUN automático. El STEP manual sigue disponible.">⏸ PAUSE</button>
        <button @click="cpu.reset" :disabled="cpu.isViewer()" class="btn-reset" title="Borra TODO: kernel, procesos, log, consola. Vuelve al estado inicial sin programa cargado.">↻ RESET</button>
        <button @click="cpu.clearLog" class="btn-clear" title="Limpia el log de eventos y la consola, sin tocar el programa cargado ni el estado del CPU.">⌧ CLEAR</button>
        <button v-if="!cpu.isViewer()" @click="$emit('open-share')" class="btn-share" :class="{ 'is-sharing': cpu.sessionInfo.view_token }" title="Comparta esta sesión en modo solo lectura. Genera un enlace que cualquiera puede abrir para ver lo que usted hace.">
          {{ cpu.sessionInfo.view_token ? '🔗 COMPARTIDA' : '🔗 COMPARTIR' }}
        </button>
        <button @click="$emit('open-glossary')" class="btn-glossary" title="Glosario: definiciones de IRQ, PL, flags, registros, scheduler, etc.">? GLOSARIO</button>
      </div>

      <div class="config">
        <label class="cfg-item" title="Velocidad del modo RUN automático. Más Hz = pasos por segundo más rápidos.">
          <span>Hz</span>
          <input type="number" name="step_hz" min="1" max="60" v-model.number="cpu.stepHz" aria-label="Hz del modo RUN automático" />
        </label>
        <label class="cfg-item" title="Quantum: cuántos ticks del CPU corre un proceso antes de que el timer dispare IRQ 0x20 y el scheduler le ceda el CPU a otro proceso. 0 = timer apagado.">
          <span>Quantum</span>
          <input
            type="number"
            name="timer_quantum"
            min="0"
            max="200"
            :value="cpu.timerQuantum"
            aria-label="Quantum del timer (ticks por proceso)"
            @change="(e) => cpu.setQuantum(Number((e.target as HTMLInputElement).value))"
          />
        </label>
      </div>

      <div class="pills">
        <span
          class="pill clickable"
          :class="modeClass"
          :title="(cpu.mode === 3 ? 'PL3 / USER MODE: el proceso está corriendo en modo usuario. No puede acceder a memoria kernel ni ejecutar instrucciones privilegiadas.' : 'PL0 / KERNEL MODE: el código corriendo tiene privilegios totales. Solo se entra por IRQ, INT o excepción (page fault, divide error, etc.) — el HW cambia el modo automáticamente.') + '\n\n— click para más info —'"
          @click="cpu.openGlossary(cpu.mode === 3 ? 'USER MODE' : 'KERNEL MODE')"
        >MODE: {{ modeLabel }}</span>
        <span
          class="pill clickable"
          title="Contador de ticks del CPU. Incrementa con cada Step. El timer dispara una IRQ cada `Quantum` ticks.&#10;&#10;— click para más info —"
          @click="cpu.openGlossary('Quantum')"
        >TICK: {{ cpu.tick }}</span>
        <span v-if="cpu.halted" class="pill halted clickable" title="CPU detenido por HLT, sys_exit del último proceso, o page fault sin más procesos. Se reactiva si llega una IRQ y IF=1.&#10;&#10;— click para más info —" @click="cpu.openGlossary('HLT')">HALTED</span>
        <span v-if="cpu.running" class="pill" style="color: var(--green); border-color: var(--green)" title="Modo RUN activo: el frontend está pidiendo Step a Hz por segundo.">RUNNING</span>
        <span class="pill" :style="{ color: cpu.connected ? 'var(--green)' : 'var(--red)', borderColor: cpu.connected ? 'var(--green)' : 'var(--red)' }" title="WebSocket: conexión con el backend Go. OFF = se perdió la conexión, refresque la página.">
          WS: {{ cpu.connected ? 'ON' : 'OFF' }}
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.topbar {
  background: rgba(255, 179, 71, 0.04);
  border-bottom: 1px solid var(--border-strong);
  padding: 8px 14px;
}

.topbar-row {
  display: flex;
  gap: 14px;
  align-items: center;
  flex-wrap: wrap;
}

.program-picker {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 220px;
  max-width: 320px;
  flex: 1;
}

.program-picker select {
  /* Hereda los estilos globales pero acotamos el ancho. */
  width: 100%;
  font-weight: 700;
}

.program-desc {
  font-size: 11px;
  color: var(--text-dim);
  font-style: italic;
  line-height: 1.3;
  /* Truncamos a 2 líneas por si la descripción es larga. */
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.actions {
  display: flex;
  gap: 6px;
  align-items: center;
}

.btn-reset {
  color: var(--red-soft);
  border-color: var(--red-soft);
}

.btn-reset:hover {
  background: rgba(255, 122, 122, 0.12);
  box-shadow: 0 0 8px rgba(255, 122, 122, 0.25);
}

.btn-clear {
  color: var(--text-dim);
  border-color: var(--text-muted);
}

.btn-clear:hover {
  color: var(--text);
  border-color: var(--text-dim);
}

.btn-glossary {
  color: var(--cyan);
  border-color: var(--cyan);
}

.btn-glossary:hover {
  background: rgba(106, 212, 255, 0.12);
  box-shadow: 0 0 8px rgba(106, 212, 255, 0.3);
}

.btn-share {
  color: var(--green);
  border-color: var(--green);
}

.btn-share:hover {
  background: rgba(127, 255, 170, 0.12);
  box-shadow: 0 0 8px rgba(127, 255, 170, 0.35);
}

.btn-share.is-sharing {
  background: rgba(127, 255, 170, 0.18);
  box-shadow: 0 0 6px rgba(127, 255, 170, 0.4);
}

/* Input compacto de alias — visible para owners. Se acopla al lado del
   ProgramPicker. En mobile pasa a una línea propia. */
.alias-wrap {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: rgba(106, 212, 255, 0.04);
  border: 1px solid var(--border);
  padding: 0 10px;
  min-height: 32px;
  max-width: 160px;
}

.alias-wrap:focus-within {
  border-color: var(--cyan);
  box-shadow: 0 0 6px rgba(106, 212, 255, 0.25);
}

.alias-icon {
  color: var(--cyan);
  font-size: 11px;
}

.alias-input {
  background: transparent;
  border: 0;
  color: var(--cyan);
  font-family: inherit;
  font-size: 13px;
  padding: 4px 0;
  width: 100%;
  min-width: 0;
  outline: none;
  text-transform: lowercase;
}

.alias-input::placeholder {
  color: var(--text-muted);
  font-style: italic;
  letter-spacing: 0;
}

.btn-login-mini {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  font-size: 12px;
  padding: 2px 6px;
  min-width: 22px;
  letter-spacing: 0;
}

.btn-login-mini:hover {
  color: var(--amber);
  border-color: var(--amber);
  background: rgba(255, 179, 71, 0.1);
}

/* Read-only identity pill shown when a user is authenticated. */
.user-wrap {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: rgba(255, 179, 71, 0.06);
  border: 1px solid var(--amber);
  padding: 0 8px 0 10px;
  min-height: 32px;
  max-width: 220px;
  color: var(--amber);
  font-size: 13px;
}

.user-icon {
  font-size: 12px;
}

.user-name {
  text-overflow: ellipsis;
  overflow: hidden;
  white-space: nowrap;
  max-width: 140px;
}

.btn-logout {
  background: transparent;
  border: 1px solid transparent;
  color: var(--text-dim);
  font-size: 14px;
  line-height: 1;
  padding: 0 4px;
  min-width: 20px;
  letter-spacing: 0;
}

.btn-logout:hover {
  color: var(--red-soft);
  border-color: var(--red-soft);
}

.config {
  display: flex;
  gap: 8px;
}

.cfg-item {
  display: flex;
  align-items: center;
  gap: 4px;
  color: var(--text-dim);
  font-size: 13px;
  text-transform: uppercase;
  letter-spacing: 1px;
}

.cfg-item input {
  width: 50px;
  background: rgba(0, 0, 0, 0.4);
  border: 1px solid var(--border);
  color: var(--amber);
  padding: 4px;
  font-family: inherit;
  font-size: 14px;
}

.pills {
  display: flex;
  gap: 6px;
  align-items: center;
  flex-wrap: wrap;
  margin-left: auto;
}

.pill.clickable {
  cursor: pointer;
  transition: all 0.15s ease;
}

.pill.clickable:hover {
  background: rgba(106, 212, 255, 0.1);
  filter: brightness(1.2);
}

/* ─── MOBILE (≤ 768px) ──────────────────────────────────────────────────
   El topbar pasa a un layout vertical de filas: program picker full-width
   arriba, acciones en grid de 3 columnas (LOAD/STEP/RUN se ven juntos),
   config + pills al final, también en grid. */
@media (max-width: 768px) {
  .topbar {
    padding: 6px 8px;
  }

  .topbar-row {
    gap: 6px;
  }

  .alias-wrap {
    flex: 1 1 100%;
    max-width: 100%;
    margin-bottom: 2px;
    min-height: 38px;
  }

  .alias-input {
    font-size: 14px;
  }

  .program-picker {
    flex: 1 1 100%;
    min-width: 100%;
    max-width: 100%;
    margin-bottom: 2px;
  }

  .actions {
    flex: 1 1 100%;
    gap: 4px;
    flex-wrap: wrap;
    justify-content: space-between;
  }

  .actions button {
    flex: 1 1 calc(33.333% - 4px);
    min-width: 0;
    font-size: 11px;
    padding: 10px 4px;
    letter-spacing: 0.5px;
    min-height: 44px;
  }

  .config {
    flex: 1 1 auto;
    gap: 10px;
  }

  .cfg-item {
    font-size: 11px;
  }

  .cfg-item input {
    width: 56px;
    min-height: 36px;
    padding: 4px 6px;
    font-size: 13px;
  }

  .pills {
    flex: 1 1 100%;
    margin-left: 0;
    justify-content: flex-start;
    gap: 4px;
  }

  .pill {
    font-size: 11px;
    padding: 4px 7px;
    letter-spacing: 0.5px;
  }
}
</style>
