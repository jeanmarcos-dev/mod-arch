<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { apiFetch } from '../../lib/api';
import { useAdminStore } from '../../stores/admin';
import { useToastStore } from '../../stores/toast';
import Card from '../../components/ui/Card.vue';

// API contract mirrors internal/tutor/config.go MaskedConfig.
interface MaskedConfig {
  enabled: boolean;
  llm_provider: string;
  llm_model: string;
  llm_base_url: string;
  llm_rpm: number;
  mode: string;
  embedder_provider: string;
  ollama_host: string;
  ollama_embed_model: string;
  history_window_turns: number;
  gemini_api_key: string;        has_gemini_key: boolean;
  deepseek_api_key: string;      has_deepseek_key: boolean;
  groq_api_key: string;          has_groq_key: boolean;
  openai_compat_api_key: string; has_openai_compat_key: boolean;
  jina_api_key: string;          has_jina_key: boolean;
}
interface ConfigResponse {
  config: MaskedConfig;
  tutor_running: boolean;
  corpus_version: number;
  reload_error?: string;
}

const admin = useAdminStore();
const toast = useToastStore();
const config = ref<MaskedConfig | null>(null);
const tutorRunning = ref(false);
const corpusVersion = ref(0);
const loading = ref(false);
const saving = ref(false);

// Draft form: holds user edits not yet sent. API key fields stay empty
// unless the admin types a new one (so the persisted key is not overwritten
// by accident).
const draft = ref({
  enabled: true,
  llm_provider: 'gemini',
  llm_model: '',
  llm_base_url: '',
  llm_rpm: 12,
  mode: 'direct',
  embedder_provider: 'ollama',
  ollama_host: 'http://localhost:11434',
  ollama_embed_model: 'bge-m3',
  history_window_turns: 3,
  gemini_api_key: '',
  deepseek_api_key: '',
  groq_api_key: '',
  openai_compat_api_key: '',
  jina_api_key: '',
});

// Whether to show actual key chars instead of dots (per-field toggle).
const showKey = ref<Record<string, boolean>>({});

async function load() {
  loading.value = true;
  try {
    const r = await apiFetch('/admin/api/tutor/config');
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    const data: ConfigResponse = await r.json();
    config.value = data.config;
    tutorRunning.value = data.tutor_running;
    corpusVersion.value = data.corpus_version;
    // Populate the draft with persisted non-secret fields. Keys stay blank.
    draft.value = {
      enabled: data.config.enabled,
      llm_provider: data.config.llm_provider || 'gemini',
      llm_model: data.config.llm_model,
      llm_base_url: data.config.llm_base_url,
      llm_rpm: data.config.llm_rpm || 12,
      mode: data.config.mode || 'direct',
      embedder_provider: data.config.embedder_provider || 'ollama',
      ollama_host: data.config.ollama_host || 'http://localhost:11434',
      ollama_embed_model: data.config.ollama_embed_model || 'bge-m3',
      history_window_turns:
        typeof data.config.history_window_turns === 'number' ? data.config.history_window_turns : 3,
      gemini_api_key: '',
      deepseek_api_key: '',
      groq_api_key: '',
      openai_compat_api_key: '',
      jina_api_key: '',
    };
  } catch (e) {
    toast.err('No se pudo cargar la configuración: ' + String(e));
  } finally {
    loading.value = false;
  }
}

async function save(reload = true) {
  saving.value = true;
  try {
    // Send only fields that differ from persisted or are non-empty. API
    // keys are sent only when the admin typed something new.
    const patch: Record<string, unknown> = {
      enabled: draft.value.enabled,
      llm_provider: draft.value.llm_provider,
      llm_model: draft.value.llm_model,
      llm_base_url: draft.value.llm_base_url,
      llm_rpm: draft.value.llm_rpm,
      mode: draft.value.mode,
      embedder_provider: draft.value.embedder_provider,
      ollama_host: draft.value.ollama_host,
      ollama_embed_model: draft.value.ollama_embed_model,
      history_window_turns: draft.value.history_window_turns,
    };
    if (draft.value.gemini_api_key)        patch.gemini_api_key = draft.value.gemini_api_key;
    if (draft.value.deepseek_api_key)      patch.deepseek_api_key = draft.value.deepseek_api_key;
    if (draft.value.groq_api_key)          patch.groq_api_key = draft.value.groq_api_key;
    if (draft.value.openai_compat_api_key) patch.openai_compat_api_key = draft.value.openai_compat_api_key;
    if (draft.value.jina_api_key)          patch.jina_api_key = draft.value.jina_api_key;

    const r = await apiFetch(`/admin/api/tutor/config?reload=${reload}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    });
    if (!r.ok) {
      const text = await r.text();
      throw new Error(text || `HTTP ${r.status}`);
    }
    const data: ConfigResponse = await r.json();
    config.value = data.config;
    tutorRunning.value = data.tutor_running;
    if (data.reload_error) {
      toast.err('Configuración guardada pero el tutor no pudo recargar: ' + data.reload_error);
    } else if (reload) {
      toast.ok('Configuración guardada y tutor recargado.');
    } else {
      toast.ok('Configuración guardada (sin recargar).');
    }
    // Limpia los campos de keys para que no queden visibles tras guardar.
    draft.value.gemini_api_key = '';
    draft.value.deepseek_api_key = '';
    draft.value.groq_api_key = '';
    draft.value.openai_compat_api_key = '';
    draft.value.jina_api_key = '';
  } catch (e) {
    toast.err('Error al guardar: ' + String(e));
  } finally {
    saving.value = false;
  }
}

async function reload() {
  saving.value = true;
  try {
    const r = await apiFetch('/admin/api/tutor/reload', { method: 'POST' });
    if (!r.ok) {
      const text = await r.text();
      throw new Error(text || `HTTP ${r.status}`);
    }
    const data = await r.json();
    tutorRunning.value = data.tutor_running;
    if (tutorRunning.value) {
      toast.ok('Tutor recargado correctamente.');
    } else {
      toast.err('El tutor no pudo arrancar — revise la configuración.');
    }
  } catch (e) {
    toast.err('Error al recargar: ' + String(e));
  } finally {
    saving.value = false;
  }
}

const provider = computed(() => draft.value.llm_provider);
const needsBaseURL = computed(() => provider.value === 'openai_compat');
const needsJina = computed(() => draft.value.embedder_provider === 'jina');

function toggleKey(field: string) {
  showKey.value = { ...showKey.value, [field]: !showKey.value[field] };
}

function placeholder(masked: string, has: boolean): string {
  if (!has) return 'no configurada';
  return masked || '••••••••';
}

onMounted(load);
</script>

<template>
  <div class="tutor-cfg">
    <header class="head">
      <h1>Tutor — configuración</h1>
      <div class="status">
        <span class="badge" :class="{ on: tutorRunning, off: !tutorRunning }">
          {{ tutorRunning ? 'EN MARCHA' : 'DETENIDO' }}
        </span>
        <span v-if="corpusVersion" class="cv">corpus v{{ corpusVersion }}</span>
      </div>
    </header>
    <p class="subtitle">Conectado como <strong>{{ admin.user }}</strong></p>

    <Card title="Maestro" :elevation="1">
      <label class="row toggle">
        <input type="checkbox" v-model="draft.enabled" />
        <span>{{ draft.enabled ? 'Tutor habilitado' : 'Tutor deshabilitado' }}</span>
      </label>
      <p class="hint">Cuando está deshabilitado, el panel de tutor sigue visible pero responde "desactivado por el administrador" a toda pregunta.</p>
    </Card>

    <Card title="Modelo de lenguaje (LLM)" :elevation="1">
      <div class="grid">
        <label class="field">
          <span>Proveedor</span>
          <select v-model="draft.llm_provider">
            <option value="gemini">Gemini (Google AI Studio) — gratis 15 RPM</option>
            <option value="deepseek">DeepSeek — $5 trial</option>
            <option value="groq">Groq — 30 RPM (TPM apretado)</option>
            <option value="openai_compat">OpenAI-compat (custom)</option>
          </select>
        </label>
        <label class="field">
          <span>Modelo (vacío = default del proveedor)</span>
          <input type="text" v-model="draft.llm_model" placeholder="gemini-2.5-flash-lite" />
        </label>
        <label v-if="needsBaseURL" class="field">
          <span>Base URL</span>
          <input type="text" v-model="draft.llm_base_url" placeholder="https://openrouter.ai/api/v1/chat/completions" />
        </label>
        <label class="field">
          <span>Rate limit (RPM)</span>
          <input type="number" v-model.number="draft.llm_rpm" min="0" max="120" />
          <small class="hint inline">0 desactiva el bucket. Gemini Flash-Lite: 15 RPM.</small>
        </label>
        <label class="field">
          <span>Modo del tutor</span>
          <select v-model="draft.mode">
            <option value="direct">Directo — responde con la solución</option>
            <option value="socratic">Socrático — responde con preguntas guiadas</option>
          </select>
        </label>
        <label class="field">
          <span>Memoria conversacional (turnos)</span>
          <input type="number" v-model.number="draft.history_window_turns" min="0" max="10" />
          <small class="hint inline">Cantidad de turnos previos que el tutor recuerda. 0 desactiva el seguimiento y vuelve al modo por pregunta aislada.</small>
        </label>
      </div>
    </Card>

    <Card title="API keys" :elevation="1">
      <p class="hint">Las keys se guardan encriptadas (AES-GCM con SESSION_SECRET). Dejar el campo vacío para conservar la key actual.</p>
      <div class="grid">
        <div class="field key-field">
          <span class="label-row">
            <span>Gemini API key</span>
            <code class="masked">{{ placeholder(config?.gemini_api_key ?? '', config?.has_gemini_key ?? false) }}</code>
          </span>
          <div class="key-input">
            <input :type="showKey.gemini ? 'text' : 'password'" v-model="draft.gemini_api_key" placeholder="(pegar nueva key)" autocomplete="off" />
            <button type="button" @click="toggleKey('gemini')">{{ showKey.gemini ? '🙈' : '👁' }}</button>
          </div>
        </div>
        <div class="field key-field">
          <span class="label-row">
            <span>DeepSeek API key</span>
            <code class="masked">{{ placeholder(config?.deepseek_api_key ?? '', config?.has_deepseek_key ?? false) }}</code>
          </span>
          <div class="key-input">
            <input :type="showKey.deepseek ? 'text' : 'password'" v-model="draft.deepseek_api_key" placeholder="(pegar nueva key)" autocomplete="off" />
            <button type="button" @click="toggleKey('deepseek')">{{ showKey.deepseek ? '🙈' : '👁' }}</button>
          </div>
        </div>
        <div class="field key-field">
          <span class="label-row">
            <span>Groq API key</span>
            <code class="masked">{{ placeholder(config?.groq_api_key ?? '', config?.has_groq_key ?? false) }}</code>
          </span>
          <div class="key-input">
            <input :type="showKey.groq ? 'text' : 'password'" v-model="draft.groq_api_key" placeholder="(pegar nueva key)" autocomplete="off" />
            <button type="button" @click="toggleKey('groq')">{{ showKey.groq ? '🙈' : '👁' }}</button>
          </div>
        </div>
        <div v-if="needsBaseURL" class="field key-field">
          <span class="label-row">
            <span>OpenAI-compat key</span>
            <code class="masked">{{ placeholder(config?.openai_compat_api_key ?? '', config?.has_openai_compat_key ?? false) }}</code>
          </span>
          <div class="key-input">
            <input :type="showKey.openai_compat ? 'text' : 'password'" v-model="draft.openai_compat_api_key" placeholder="(pegar nueva key)" autocomplete="off" />
            <button type="button" @click="toggleKey('openai_compat')">{{ showKey.openai_compat ? '🙈' : '👁' }}</button>
          </div>
        </div>
      </div>
    </Card>

    <Card title="Embeddings (RAG)" :elevation="1">
      <div class="grid">
        <label class="field">
          <span>Proveedor de embeddings</span>
          <select v-model="draft.embedder_provider">
            <option value="ollama">Ollama (self-host)</option>
            <option value="jina">Jina API</option>
          </select>
        </label>
        <label v-if="draft.embedder_provider === 'ollama'" class="field">
          <span>Host de Ollama</span>
          <input type="text" v-model="draft.ollama_host" placeholder="http://localhost:11434" />
        </label>
        <label v-if="draft.embedder_provider === 'ollama'" class="field">
          <span>Modelo de embeddings</span>
          <input type="text" v-model="draft.ollama_embed_model" placeholder="bge-m3" />
        </label>
        <div v-if="needsJina" class="field key-field">
          <span class="label-row">
            <span>Jina API key</span>
            <code class="masked">{{ placeholder(config?.jina_api_key ?? '', config?.has_jina_key ?? false) }}</code>
          </span>
          <div class="key-input">
            <input :type="showKey.jina ? 'text' : 'password'" v-model="draft.jina_api_key" placeholder="(pegar nueva key)" autocomplete="off" />
            <button type="button" @click="toggleKey('jina')">{{ showKey.jina ? '🙈' : '👁' }}</button>
          </div>
        </div>
      </div>
    </Card>

    <div class="actions">
      <button class="btn primary" :disabled="saving || loading" @click="save(true)">
        {{ saving ? 'Guardando…' : 'Guardar y recargar tutor' }}
      </button>
      <button class="btn ghost" :disabled="saving || loading" @click="save(false)">
        Guardar sin recargar
      </button>
      <button class="btn ghost" :disabled="saving || loading" @click="reload">
        Solo recargar
      </button>
      <button class="btn ghost" :disabled="loading" @click="load">↻ Refrescar</button>
    </div>
  </div>
</template>

<style scoped>
.tutor-cfg { display: flex; flex-direction: column; gap: var(--space-4); }
.head { display: flex; justify-content: space-between; align-items: center; }
.head h1 {
  font-family: var(--font-display);
  font-size: 18px;
  letter-spacing: 4px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  font-weight: normal;
  margin: 0;
}
.status { display: flex; align-items: center; gap: 0.5rem; }
.badge {
  padding: 0.2rem 0.6rem;
  border-radius: 0.3rem;
  font-size: 10px;
  letter-spacing: 1.5px;
  font-family: 'JetBrains Mono', monospace;
  font-weight: 600;
}
.badge.on  { background: rgba(74, 222, 128, 0.15); color: #86efac; border: 1px solid rgba(74,222,128,0.4); }
.badge.off { background: rgba(239, 68, 68, 0.15);  color: #fca5a5; border: 1px solid rgba(239,68,68,0.4); }
.cv { font-size: 11px; color: var(--text-dim); font-family: 'JetBrains Mono', monospace; }
.subtitle { font-size: 12px; color: var(--text-dim); margin: 0; }
.subtitle strong { color: var(--amber); letter-spacing: 1px; }

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 0.85rem;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  font-size: 12px;
}
.field > span:not(.label-row) { color: var(--text-dim); letter-spacing: 0.5px; text-transform: uppercase; font-size: 10px; }
.field input, .field select {
  background: var(--surface-overlay);
  border: 1px solid rgba(245, 158, 11, 0.3);
  color: var(--text);
  padding: 0.45rem 0.55rem;
  font-family: inherit;
  font-size: 13px;
  border-radius: 0.25rem;
  outline: none;
}
.field input:focus, .field select:focus { border-color: var(--amber); }
.field small.inline { font-size: 10px; color: var(--text-dim); }

.label-row {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 0.5rem;
}
.label-row > span:first-child { color: var(--text-dim); letter-spacing: 0.5px; text-transform: uppercase; font-size: 10px; }
.masked { font-family: 'JetBrains Mono', monospace; font-size: 11px; color: var(--text-dim); letter-spacing: 1px; }

.key-input { display: flex; gap: 0.3rem; }
.key-input input { flex: 1; }
.key-input button {
  background: transparent;
  border: 1px solid rgba(245, 158, 11, 0.3);
  padding: 0 0.5rem;
  cursor: pointer;
  border-radius: 0.25rem;
  color: var(--amber);
}
.key-input button:hover { background: rgba(245, 158, 11, 0.1); }

.row.toggle { display: flex; align-items: center; gap: 0.5rem; font-size: 13px; color: var(--text); }
.row.toggle input { width: 1rem; height: 1rem; cursor: pointer; }
.hint { font-size: 11px; color: var(--text-dim); margin: 0.4rem 0 0; line-height: 1.4; }

.actions { display: flex; gap: 0.6rem; flex-wrap: wrap; }
.btn {
  padding: 0.5rem 0.9rem;
  border-radius: 0.3rem;
  cursor: pointer;
  font-family: inherit;
  font-size: 12px;
  letter-spacing: 0.5px;
  border: 1px solid;
}
.btn.primary {
  background: rgba(245, 158, 11, 0.18);
  border-color: var(--amber);
  color: var(--amber);
}
.btn.primary:hover:not(:disabled) { background: rgba(245, 158, 11, 0.3); }
.btn.ghost {
  background: transparent;
  border-color: rgba(245, 158, 11, 0.3);
  color: var(--text);
}
.btn.ghost:hover:not(:disabled) { background: rgba(245, 158, 11, 0.06); }
.btn:disabled { opacity: 0.45; cursor: not-allowed; }
</style>
