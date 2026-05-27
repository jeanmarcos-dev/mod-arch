<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue';
import { BotMessageSquare, Send, Trash2, Minus, Maximize2, Minimize2 } from '@lucide/vue';

import TutorMessage from './TutorMessage.vue';
import { useTutor } from '../../composables/useTutor';
import { useTutorStore, type TutorCitation } from '../../stores/tutor';

const emit = defineEmits<{ (e: 'close'): void; (e: 'cite', citation: TutorCitation): void }>();

const tutor = useTutorStore();
const { ask } = useTutor();
const draft = ref('');
const listRef = ref<HTMLElement | null>(null);
const isExpanded = computed(() => tutor.displayMode === 'expanded');

function submit() {
  const q = draft.value.trim();
  if (!q || tutor.sending) return;
  ask(q);
  draft.value = '';
}

function clear() {
  if (confirm('¿Borrar la conversación actual con el tutor?')) {
    tutor.clear();
  }
}

function onCite(citation: TutorCitation) {
  emit('cite', citation);
}

function scrollToBottom() {
  if (!listRef.value) return;
  listRef.value.scrollTop = listRef.value.scrollHeight;
}

watch(() => tutor.messages.length, () => nextTick(scrollToBottom));
onMounted(scrollToBottom);
</script>

<template>
  <aside
    class="tutor-panel"
    :class="{ 'is-expanded': isExpanded }"
    aria-label="Tutor de modos-cpu"
  >
    <header class="head">
      <div class="title">
        <div class="title-icon">
          <BotMessageSquare :size="18" :stroke-width="1.8" />
          <span class="status-led" aria-hidden="true" />
        </div>
        <div class="title-text">
          <span class="title-main">TUTOR</span>
          <span class="title-sub">agente de sistemas operativos</span>
        </div>
      </div>
      <div class="actions">
        <button
          v-if="tutor.messages.length > 0"
          class="icon-btn"
          @click="clear"
          title="Borrar conversación"
          aria-label="Borrar conversación"
        >
          <Trash2 :size="14" :stroke-width="2" />
        </button>
        <button
          class="icon-btn"
          @click="tutor.toggleDisplayMode()"
          :title="isExpanded ? 'Contraer panel' : 'Expandir panel'"
          :aria-label="isExpanded ? 'Contraer panel' : 'Expandir panel'"
        >
          <Minimize2 v-if="isExpanded" :size="14" :stroke-width="2" />
          <Maximize2 v-else :size="14" :stroke-width="2" />
        </button>
        <button class="icon-btn" @click="emit('close')" title="Minimizar tutor" aria-label="Minimizar tutor">
          <Minus :size="14" :stroke-width="2.5" />
        </button>
      </div>
    </header>

    <div v-if="tutor.messages.length === 0" class="empty">
      <div class="empty-avatar">
        <BotMessageSquare :size="28" :stroke-width="1.4" />
      </div>
      <p class="empty-hello">¿En qué puedo ayudarle?</p>
      <p class="empty-sub">Pregunte sobre lo que ve en el simulador.</p>
      <ul class="hints">
        <li><button class="hint" @click="draft = '¿Qué significa que el CPU esté en modo PL3?'">¿qué es PL3?</button></li>
        <li><button class="hint" @click="draft = '¿Por qué cambió de proceso en el último context switch?'">¿por qué cambió de proceso?</button></li>
        <li><button class="hint" @click="draft = '¿Para qué sirve la cola de READY del scheduler?'">cola READY</button></li>
      </ul>
    </div>

    <div v-else ref="listRef" class="list">
      <TutorMessage
        v-for="m in tutor.messages"
        :key="m.id"
        :msg="m"
        @cite="onCite"
      />
    </div>

    <form class="composer" @submit.prevent="submit">
      <textarea
        v-model="draft"
        name="tutor_question"
        aria-label="Pregunta al tutor"
        placeholder="Escriba su pregunta…"
        rows="2"
        :disabled="tutor.sending"
        @keydown.enter.exact.prevent="submit"
      />
      <button
        type="submit"
        class="send-btn"
        :disabled="tutor.sending || !draft.trim()"
        aria-label="Enviar pregunta"
      >
        <Send :size="16" :stroke-width="2" />
      </button>
    </form>
  </aside>
</template>

<style scoped>
.tutor-panel {
  /* TutorMessage reads --bubble-max-width to size its .bubble. We override it
     in expanded mode so messages can use the extra horizontal space. */
  --bubble-max-width: 32ch;
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  /* Fully opaque background. The previous gradient + backdrop-filter combo
     read as translucent on top of the simulator and let narrator banners
     bleed through visually; an opaque surface guarantees the dialog covers
     whatever is underneath. */
  background: var(--surface-1);
  border: 1px solid rgba(var(--accent-rgb), 0.5);
  border-radius: 0.75rem;
  color: var(--text);
  font-family: inherit;
  box-shadow:
    0 20px 50px rgba(0, 0, 0, 0.75),
    0 0 0 1px rgba(var(--accent-rgb), 0.08),
    0 0 30px rgba(var(--accent-rgb), 0.15);
  overflow: hidden;
}
.tutor-panel.is-expanded { --bubble-max-width: 64ch; }

/* ── HEADER ──────────────────────────────────────────────────────────── */
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.7rem 0.85rem;
  background:
    linear-gradient(180deg, rgba(var(--accent-rgb), 0.12) 0%, rgba(var(--accent-rgb), 0.04) 100%);
  border-bottom: 1px solid rgba(var(--accent-rgb), 0.35);
}
.title {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  min-width: 0;
}
.title-icon {
  position: relative;
  width: 2rem;
  height: 2rem;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: radial-gradient(circle at 30% 30%, rgba(var(--accent-rgb), 0.45), rgba(var(--accent-rgb), 0.12) 70%);
  border: 1px solid rgba(var(--accent-rgb), 0.7);
  color: var(--accent);
  flex-shrink: 0;
}
.status-led {
  position: absolute;
  right: -2px;
  bottom: -2px;
  width: 0.55rem;
  height: 0.55rem;
  border-radius: 50%;
  background: var(--green);
  border: 1.5px solid var(--surface-0);
  box-shadow: 0 0 6px rgba(127, 255, 170, 0.8);
}
.title-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.title-main {
  font-family: var(--font-display);
  font-size: 0.95rem;
  letter-spacing: 0.25em;
  color: var(--accent);
  text-shadow: 0 0 10px rgba(var(--accent-rgb), 0.35);
}
.title-sub {
  font-size: 0.65rem;
  color: var(--text-muted);
  letter-spacing: 0.12em;
  text-transform: uppercase;
}
.actions {
  display: flex;
  gap: 0.3rem;
}
.icon-btn {
  background: transparent;
  border: 1px solid transparent;
  color: var(--text-muted);
  padding: 0.4rem;
  border-radius: 0.3rem;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: all 120ms ease;
}
.icon-btn:hover {
  color: var(--accent);
  border-color: rgba(var(--accent-rgb), 0.4);
  background: rgba(var(--accent-rgb), 0.08);
}

/* ── EMPTY STATE ─────────────────────────────────────────────────────── */
.empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 1.5rem 1rem;
  text-align: center;
  gap: 0.4rem;
}
.empty-avatar {
  width: 3.5rem;
  height: 3.5rem;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 0.5rem;
  background: radial-gradient(circle at 30% 30%, rgba(var(--accent-rgb), 0.35), rgba(var(--accent-rgb), 0.05) 70%);
  border: 1px solid rgba(var(--accent-rgb), 0.5);
  color: var(--accent);
  box-shadow: 0 0 24px rgba(var(--accent-rgb), 0.25);
}
.empty-hello {
  margin: 0;
  font-size: 1rem;
  color: var(--accent);
  font-weight: 500;
}
.empty-sub {
  margin: 0 0 0.85rem 0;
  font-size: 0.78rem;
  color: var(--text-muted);
}
.hints {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  width: 100%;
  max-width: 18rem;
}
.hint {
  width: 100%;
  background: rgba(var(--accent-rgb), 0.06);
  border: 1px solid rgba(var(--accent-rgb), 0.25);
  color: var(--accent);
  padding: 0.5rem 0.7rem;
  border-radius: 0.45rem;
  font-family: inherit;
  font-size: 0.78rem;
  cursor: pointer;
  text-align: left;
  transition: all 120ms ease;
}
.hint:hover {
  background: rgba(var(--accent-rgb), 0.15);
  border-color: rgba(var(--accent-rgb), 0.55);
  transform: translateX(2px);
}

/* ── MESSAGE LIST ────────────────────────────────────────────────────── */
.list {
  flex: 1;
  overflow-y: auto;
  padding: 0.5rem 0;
  scrollbar-width: thin;
  scrollbar-color: rgba(var(--accent-rgb), 0.3) transparent;
}
.list::-webkit-scrollbar { width: 6px; }
.list::-webkit-scrollbar-thumb {
  background: rgba(var(--accent-rgb), 0.3);
  border-radius: 3px;
}

/* ── COMPOSER ────────────────────────────────────────────────────────── */
.composer {
  display: flex;
  gap: 0.45rem;
  padding: 0.65rem 0.75rem;
  border-top: 1px solid rgba(var(--accent-rgb), 0.25);
  background: var(--surface-overlay-strong);
}
.composer textarea {
  flex: 1;
  background: var(--surface-overlay-strong);
  border: 1px solid rgba(var(--accent-rgb), 0.35);
  color: var(--text);
  padding: 0.55rem 0.7rem;
  border-radius: 0.4rem;
  font-family: inherit;
  font-size: 0.85rem;
  line-height: 1.4;
  resize: none;
  outline: none;
  transition: border-color 120ms ease, box-shadow 120ms ease;
}
.composer textarea::placeholder {
  color: var(--text-muted);
}
.composer textarea:focus {
  border-color: var(--accent);
  box-shadow: 0 0 0 3px rgba(var(--accent-rgb), 0.15);
}
.composer textarea:disabled {
  opacity: 0.55;
  cursor: wait;
}
.send-btn {
  background: radial-gradient(circle at 30% 30%, rgba(var(--accent-rgb), 0.5), rgba(var(--accent-rgb), 0.18) 70%);
  border: 1px solid var(--accent);
  color: var(--text);
  padding: 0 0.85rem;
  border-radius: 0.4rem;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: all 120ms ease;
}
.send-btn:hover:not(:disabled) {
  background: radial-gradient(circle at 30% 30%, rgba(var(--accent-rgb), 0.65), rgba(var(--accent-rgb), 0.3) 70%);
  box-shadow: 0 0 18px rgba(var(--accent-rgb), 0.4);
}
.send-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
  background: rgba(var(--accent-rgb), 0.08);
  border-color: rgba(var(--accent-rgb), 0.2);
  color: var(--text-muted);
}
</style>
