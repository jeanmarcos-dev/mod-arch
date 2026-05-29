<script setup lang="ts">
import { computed, ref } from 'vue';
import { BotMessageSquare, User, AlertCircle, Sparkles, ExternalLink, ChevronRight, ChevronDown } from '@lucide/vue';

import type { TutorMessage, TutorCitation } from '../../stores/tutor';
import { useToastStore } from '../../stores/toast';
import { renderMarkdown } from '../../composables/useMarkdown';
import TutorCopyButton from './TutorCopyButton.vue';

const props = defineProps<{ msg: TutorMessage }>();
const emit = defineEmits<{ (e: 'cite', citation: TutorCitation): void }>();

const toast = useToastStore();

// Track which citation chips are expanded. The key is stable across rerenders
// of the same message (source + id + page).
const expandedCites = ref<Set<string>>(new Set());

function citeKey(c: TutorCitation, i: number): string {
  return `${c.source}:${c.id}:${c.page ?? i}`;
}

function toggleCite(c: TutorCitation, i: number) {
  const key = citeKey(c, i);
  const next = new Set(expandedCites.value);
  if (next.has(key)) next.delete(key); else next.add(key);
  expandedCites.value = next;
}

const isStudent = computed(() => props.msg.role === 'student');
const hasError = computed(() => !!props.msg.error);
const isPending = computed(() => props.msg.pending && !hasError.value);
// While tokens stream in, msg.pending is still true but msg.text is no
// longer empty — show the text + a small caret instead of the spinner.
const isStreaming = computed(() => isPending.value && props.msg.text.length > 0);
const renderedHtml = computed(() => renderMarkdown(props.msg.text));

async function onMarkdownClick(ev: MouseEvent) {
  const target = ev.target as HTMLElement | null;
  if (!target) return;
  const copyBtn = target.closest('[data-copy]') as HTMLElement | null;
  if (!copyBtn) return;
  const block = copyBtn.closest('.md-codeblock') as HTMLElement | null;
  const code = block?.dataset.code ?? '';
  if (!code) return;
  try {
    await navigator.clipboard.writeText(decodeHtmlEntities(code));
    toast.ok('Código copiado');
  } catch {
    toast.err('No se pudo copiar al portapapeles');
  }
}

// The raw source is stored escaped inside data-code so the attribute can hold
// arbitrary characters; decode it back before sending to the clipboard.
function decodeHtmlEntities(s: string): string {
  const el = document.createElement('textarea');
  el.innerHTML = s;
  return el.value;
}
</script>

<template>
  <div class="msg" :class="{ student: isStudent, tutor: !isStudent, error: hasError }">
    <div class="avatar" :aria-hidden="true">
      <User v-if="isStudent" :size="14" :stroke-width="2" />
      <AlertCircle v-else-if="hasError" :size="14" :stroke-width="2" />
      <BotMessageSquare v-else :size="14" :stroke-width="1.8" />
    </div>
    <div class="bubble">
      <!-- Student bubbles stay as plain text. The tutor renders markdown
           with highlight.js for code blocks, and copy buttons are wired via
           an event-delegated click handler on the rendered HTML. -->
      <div v-if="isStreaming && isStudent" class="text streaming">
        {{ msg.text }}<span class="caret" aria-hidden="true">▍</span>
      </div>
      <div v-else-if="isStreaming" class="text md streaming" @click="onMarkdownClick">
        <div class="md-body" v-html="renderedHtml" />
        <span class="caret" aria-hidden="true">▍</span>
      </div>
      <div v-else-if="isPending" class="pending">
        <span class="dot" /><span class="dot" /><span class="dot" />
        <span class="pending-label">
          <template v-if="msg.queueWaitSeconds !== undefined && msg.queueWaitSeconds > 0">
            en cola — espera <strong>~{{ msg.queueWaitSeconds }}s</strong>
          </template>
          <template v-else-if="msg.toolInFlight">consultando: <code>{{ msg.toolInFlight }}</code>…</template>
          <template v-else-if="msg.toolsUsed && msg.toolsUsed.length">razonando con datos…</template>
          <template v-else>consultando al tutor…</template>
        </span>
      </div>
      <p v-else-if="hasError" class="err-text">
        {{ msg.error }}
        <span v-if="msg.retryAfterSec" class="retry">
          (vuelva a intentar en {{ msg.retryAfterSec }}s)
        </span>
      </p>
      <p v-else-if="isStudent" class="text">{{ msg.text }}</p>
      <div v-else class="text md" v-html="renderedHtml" @click="onMarkdownClick" />

      <div v-if="msg.citations && msg.citations.length" class="cites">
        <div
          v-for="(c, i) in msg.citations"
          :key="citeKey(c, i)"
          class="cite-item"
          :class="{ expanded: expandedCites.has(citeKey(c, i)) }"
        >
          <button
            type="button"
            class="cite-chip"
            :class="'cite-' + c.source"
            :title="c.excerpt ? 'Ver extracto' : (c.source === 'glossary' ? 'Abrir en el glosario' : 'Referencia bibliográfica')"
            @click="c.excerpt ? toggleCite(c, i) : emit('cite', c)"
          >
            {{ c.label }}
            <span v-if="c.excerpt" class="cite-toggle" :aria-hidden="true">
              <ChevronDown v-if="expandedCites.has(citeKey(c, i))" :size="11" :stroke-width="2.4" />
              <ChevronRight v-else :size="11" :stroke-width="2.4" />
            </span>
          </button>
          <div v-if="c.excerpt && expandedCites.has(citeKey(c, i))" class="cite-extract">
            <p class="extract-text">"{{ c.excerpt }}"</p>
            <div class="extract-actions">
              <button
                v-if="c.source === 'glossary'"
                type="button"
                class="extract-link"
                @click="emit('cite', c)"
              >
                <ExternalLink :size="11" :stroke-width="2" />
                Abrir en el glosario
              </button>
              <button
                v-else-if="c.source === 'demos'"
                type="button"
                class="extract-link"
                @click="emit('cite', c)"
              >
                <ExternalLink :size="11" :stroke-width="2" />
                Cargar demo en el simulador
              </button>
              <span v-else-if="c.source === 'silberschatz'" class="extract-hint">
                Consulte Silberschatz <strong v-if="c.page">p. {{ c.page }}</strong> en su biblioteca.
              </span>
              <span v-else-if="c.source === 'isa'" class="extract-hint">
                ISA propio del simulador MODARCH.
              </span>
              <span v-else-if="c.source === 'docs'" class="extract-hint">
                Documentación del proyecto.
              </span>
            </div>
          </div>
        </div>
      </div>

      <div v-if="!isStudent && !isPending && !hasError" class="meta">
        <TutorCopyButton :text="msg.text" aria-label="Copiar respuesta" />
        <span v-if="msg.cacheHit" class="badge cache" title="Servido desde caché">
          <Sparkles :size="10" :stroke-width="2" /> caché
        </span>
        <span v-if="msg.degraded" class="badge degraded" title="LLM no disponible, solo se muestran pasajes">
          modo offline
        </span>
        <span v-if="msg.toolsUsed && msg.toolsUsed.length" class="badge tools" :title="msg.toolsUsed.join(', ')">
          {{ msg.toolsUsed.length }} tool{{ msg.toolsUsed.length === 1 ? '' : 's' }}
        </span>
        <span v-if="msg.latencyMs !== undefined" class="latency">{{ msg.latencyMs }} ms</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.msg {
  display: flex;
  gap: 0.5rem;
  padding: 0.5rem 0.75rem;
  align-items: flex-start;
}
.msg.student {
  flex-direction: row-reverse;
}
.avatar {
  width: 1.75rem;
  height: 1.75rem;
  border-radius: var(--radius-sm);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  border: 1px solid var(--accent);
  color: var(--accent);
  background: rgba(var(--accent-rgb), 0.08);
}
.msg.student .avatar {
  border-color: var(--green);
  color: var(--green);
  background: rgba(127, 255, 170, 0.08);
}
.msg.error .avatar {
  border-color: var(--red);
  color: var(--red);
  background: rgba(239, 68, 68, 0.1);
}
.bubble {
  background: var(--surface-overlay);
  border: 1px solid rgba(var(--accent-rgb), 0.25);
  border-radius: var(--radius-sm);
  padding: 0.5rem 0.75rem;
  /* Falls back to 32ch if the parent panel does not set a width — keeps the
     bubble usable if the component is mounted outside TutorPanel. */
  max-width: var(--bubble-max-width, 32ch);
  font-size: 0.85rem;
  line-height: 1.4;
  color: var(--text);
}
.msg.student .bubble {
  border-color: rgba(127, 255, 170, 0.3);
}
.msg.error .bubble {
  border-color: rgba(239, 68, 68, 0.5);
}
.text {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
}
.text.streaming .caret {
  display: inline-block;
  margin-left: 1px;
  color: var(--accent);
  animation: caret-blink 1s steps(2, start) infinite;
  font-weight: 700;
}
@keyframes caret-blink {
  to { visibility: hidden; }
}
.err-text {
  margin: 0;
  color: var(--red-soft);
  font-size: 0.8rem;
}
.retry {
  display: block;
  color: var(--text-muted);
  font-size: 0.75rem;
  margin-top: 0.25rem;
}
.pending {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  color: var(--text-dim);
  font-size: 0.8rem;
}
.dot {
  width: 0.35rem;
  height: 0.35rem;
  border-radius: 50%;
  background: currentColor;
  opacity: 0.4;
  animation: pulse 1.2s ease-in-out infinite;
}
.dot:nth-child(2) { animation-delay: 0.2s; }
.dot:nth-child(3) { animation-delay: 0.4s; }
@keyframes pulse {
  0%, 60%, 100% { opacity: 0.25; transform: scale(0.85); }
  30% { opacity: 1; transform: scale(1.05); }
}
.pending-label {
  margin-left: 0.35rem;
  font-style: italic;
}
.cites {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  margin-top: 0.5rem;
}
.cite-item {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  align-items: flex-start;
}
.cite-chip {
  background: rgba(var(--accent-rgb), 0.1);
  border: 1px solid rgba(var(--accent-rgb), 0.4);
  color: var(--accent);
  font-size: 0.7rem;
  padding: 0.15rem 0.55rem;
  border-radius: var(--radius-sm);
  cursor: pointer;
  font-family: inherit;
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  text-transform: none;
  letter-spacing: 0;
  transition: background 0.15s, transform 0.15s;
}
.cite-chip:hover {
  background: rgba(var(--accent-rgb), 0.22);
  transform: translateY(-1px);
}
.cite-toggle {
  display: inline-flex;
  align-items: center;
  opacity: 0.7;
  margin-left: 1px;
}
/* Differentiate by source so the user knows what the click will do.
   - glossary (default): amber, opens the glossary modal
   - silberschatz:          blue, external reference
   - demos:              green, loads the demo into the simulator
   - isa:                pink, simulator ISA reference
   - docs:               purple, project documentation */
.cite-chip.cite-silberschatz {
  background: rgba(147, 197, 253, 0.08);
  border-color: rgba(147, 197, 253, 0.5);
  color: #93c5fd;
}
.cite-chip.cite-silberschatz:hover {
  background: rgba(147, 197, 253, 0.2);
}
.cite-chip.cite-demos {
  background: rgba(127, 255, 170, 0.1);
  border-color: rgba(127, 255, 170, 0.5);
  color: #86efac;
}
.cite-chip.cite-demos:hover { background: rgba(127, 255, 170, 0.22); }
.cite-chip.cite-isa {
  background: rgba(244, 114, 182, 0.08);
  border-color: rgba(244, 114, 182, 0.5);
  color: #f0abfc;
}
.cite-chip.cite-isa:hover { background: rgba(244, 114, 182, 0.2); }
.cite-chip.cite-docs {
  background: rgba(196, 181, 253, 0.08);
  border-color: rgba(196, 181, 253, 0.5);
  color: #c4b5fd;
}
.cite-chip.cite-docs:hover { background: rgba(196, 181, 253, 0.2); }
.cite-extract {
  background: var(--surface-overlay);
  border-left: 2px solid rgba(var(--accent-rgb), 0.5);
  padding: 0.4rem 0.6rem;
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
  font-size: 0.72rem;
  color: #e2e8f0;
  max-width: 100%;
}
.cite-item.expanded .cite-chip.cite-silberschatz + .cite-extract {
  border-left-color: rgba(147, 197, 253, 0.5);
}
.extract-text {
  margin: 0 0 0.35rem;
  line-height: 1.45;
  font-style: italic;
  color: var(--accent);
}
.extract-actions {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}
.extract-link {
  background: transparent;
  border: none;
  color: #93c5fd;
  font-family: inherit;
  font-size: 0.7rem;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0;
}
.extract-link:hover { color: #bfdbfe; text-decoration: underline; }
.extract-hint { color: var(--text-muted); font-size: 0.7rem; }
.extract-hint strong { color: var(--accent); }
.meta {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  margin-top: 0.4rem;
  font-size: 0.7rem;
  color: var(--text-muted);
}
.badge {
  display: inline-flex;
  align-items: center;
  gap: 0.2rem;
  padding: 0.05rem 0.4rem;
  border-radius: var(--radius-sm);
  border: 1px solid currentColor;
}
.badge.cache { color: #93c5fd; }
.badge.degraded { color: var(--accent); }
.badge.tools { color: #c4b5fd; }
.pending-label code {
  background: rgba(196, 181, 253, 0.12);
  color: #c4b5fd;
  padding: 0 0.3rem;
  border-radius: var(--radius-sm);
  font-size: 0.72rem;
  font-family: var(--font-code);
}

/* Markdown rendering for tutor messages. The inner content is set via v-html
   so we drop scoped attributes by targeting via :deep(). */
.text.md :deep(p) {
  margin: 0 0 0.45rem;
  line-height: 1.45;
}
.text.md :deep(p:last-child) { margin-bottom: 0; }
.text.md :deep(strong) { color: var(--accent); }
.text.md :deep(em) { color: var(--accent); font-style: italic; }
.text.md :deep(ul),
.text.md :deep(ol) {
  margin: 0.2rem 0 0.5rem;
  padding-left: 1.2rem;
}
.text.md :deep(li) { margin: 0.15rem 0; }
.text.md :deep(h1),
.text.md :deep(h2),
.text.md :deep(h3) {
  font-family: var(--font-code);
  color: var(--accent);
  letter-spacing: 1px;
  margin: 0.55rem 0 0.3rem;
}
.text.md :deep(h1) { font-size: 0.95rem; }
.text.md :deep(h2) { font-size: 0.88rem; }
.text.md :deep(h3) { font-size: 0.82rem; }
.text.md :deep(a) {
  color: #93c5fd;
  text-decoration: underline;
  text-underline-offset: 2px;
}
.text.md :deep(a:hover) { color: #bfdbfe; }
.text.md :deep(blockquote) {
  border-left: 2px solid rgba(var(--accent-rgb), 0.6);
  background: rgba(var(--accent-rgb), 0.05);
  padding: 0.25rem 0.5rem;
  color: var(--accent);
  margin: 0.4rem 0;
}
.text.md :deep(code) {
  background: var(--surface-overlay);
  color: var(--accent);
  border: 1px solid rgba(var(--accent-rgb), 0.18);
  padding: 0.05rem 0.3rem;
  border-radius: var(--radius-sm);
  font-family: var(--font-code);
  font-size: 0.78rem;
}
.text.md :deep(.md-codeblock) {
  position: relative;
  margin: 0.5rem 0;
  border: 1px solid rgba(var(--accent-rgb), 0.25);
  border-radius: var(--radius-sm);
  overflow: hidden;
  background: var(--surface-overlay-strong);
}
.text.md :deep(.md-codeblock-lang) {
  position: absolute;
  top: 4px;
  left: 8px;
  font-family: var(--font-code);
  font-size: 0.65rem;
  color: var(--accent);
  opacity: 0.6;
  letter-spacing: 1px;
  text-transform: uppercase;
  pointer-events: none;
}
.text.md :deep(.md-codeblock-copy) {
  position: absolute;
  top: 4px;
  right: 6px;
  background: rgba(var(--accent-rgb), 0.1);
  border: 1px solid rgba(var(--accent-rgb), 0.35);
  color: var(--accent);
  font-family: inherit;
  font-size: 0.65rem;
  padding: 1px 6px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  letter-spacing: 0.5px;
  text-transform: uppercase;
  transition: background 0.15s, color 0.15s;
}
.text.md :deep(.md-codeblock-copy:hover) {
  background: rgba(var(--accent-rgb), 0.25);
  color: var(--accent);
}
.text.md :deep(.md-codeblock pre) {
  margin: 0;
  padding: 1.5rem 0.7rem 0.55rem;
  background: transparent;
  border: none;
  overflow-x: auto;
}
.text.md :deep(.md-codeblock code) {
  background: transparent;
  border: none;
  padding: 0;
  color: #e2e8f0;
  font-size: 0.78rem;
  line-height: 1.45;
  display: block;
}
.text.md :deep(table) {
  border-collapse: collapse;
  margin: 0.4rem 0;
  font-size: 0.78rem;
}
.text.md :deep(th),
.text.md :deep(td) {
  border: 1px solid rgba(var(--accent-rgb), 0.25);
  padding: 0.2rem 0.45rem;
  text-align: left;
}
.text.md :deep(th) {
  background: rgba(var(--accent-rgb), 0.08);
  color: var(--accent);
}

/* highlight.js colors tuned for the amber CRT palette. */
.text.md :deep(.hljs-keyword),
.text.md :deep(.hljs-built_in) { color: var(--accent); }
.text.md :deep(.hljs-string),
.text.md :deep(.hljs-attr) { color: #86efac; }
.text.md :deep(.hljs-number),
.text.md :deep(.hljs-literal) { color: #f9a8d4; }
.text.md :deep(.hljs-comment) { color: var(--text-muted); font-style: italic; }
.text.md :deep(.hljs-function),
.text.md :deep(.hljs-title) { color: #93c5fd; }
.text.md :deep(.hljs-variable),
.text.md :deep(.hljs-params) { color: var(--accent); }
.text.md :deep(.hljs-symbol),
.text.md :deep(.hljs-bullet),
.text.md :deep(.hljs-link) { color: #c4b5fd; }
.text.md :deep(.hljs-type),
.text.md :deep(.hljs-class .hljs-title) { color: #f0abfc; }

.text.md.streaming .caret {
  display: inline-block;
  margin-left: 1px;
  color: var(--accent);
  animation: caret-blink 1s steps(2, start) infinite;
  font-weight: 700;
}
</style>
