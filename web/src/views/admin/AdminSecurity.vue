<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { apiFetch } from '../../lib/api';
import { useToastStore } from '../../stores/toast';
import Card from '../../components/ui/Card.vue';

interface MaskedConfig {
  turnstile_sitekey: string;
  has_turnstile_secret: boolean;
  turnstile_secret_tag?: string;
}

const toast = useToastStore();
const config = ref<MaskedConfig | null>(null);
const loading = ref(false);
const saving = ref(false);
const showSecret = ref(false);

const draft = ref({
  turnstile_sitekey: '',
  turnstile_secret: '',
});

async function load() {
  loading.value = true;
  try {
    const r = await apiFetch('/admin/api/security/config');
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    const data = (await r.json()) as { config: MaskedConfig };
    config.value = data.config;
    draft.value.turnstile_sitekey = data.config.turnstile_sitekey;
    draft.value.turnstile_secret = '';
  } catch (e) {
    toast.err('No se pudo cargar: ' + String(e));
  } finally {
    loading.value = false;
  }
}

async function save() {
  saving.value = true;
  try {
    const patch: Record<string, unknown> = {
      turnstile_sitekey: draft.value.turnstile_sitekey,
    };
    if (draft.value.turnstile_secret) {
      patch.turnstile_secret = draft.value.turnstile_secret;
    }
    const r = await apiFetch('/admin/api/security/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    });
    if (!r.ok) {
      const txt = await r.text();
      throw new Error(txt || `HTTP ${r.status}`);
    }
    const data = (await r.json()) as { config: MaskedConfig };
    config.value = data.config;
    draft.value.turnstile_secret = '';
    toast.ok('Configuración de seguridad guardada');
  } catch (e) {
    toast.err('Error al guardar: ' + String(e));
  } finally {
    saving.value = false;
  }
}

async function clearSecret() {
  if (!confirm('¿Borrar la secret de Turnstile? El widget volverá a estar desactivado.')) return;
  saving.value = true;
  try {
    const r = await apiFetch('/admin/api/security/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ turnstile_secret: '' }),
    });
    if (!r.ok) throw new Error(await r.text());
    const data = (await r.json()) as { config: MaskedConfig };
    config.value = data.config;
    toast.ok('Secret eliminada');
  } catch (e) {
    toast.err('Error al borrar: ' + String(e));
  } finally {
    saving.value = false;
  }
}

// A half-set pair is silently ignored by the server (no widget, no verifier)
// but the admin should be warned because it usually means a paste-half-way
// mistake. We surface the warning right under the badge.
const isHalfSet = computed(() => {
  const c = config.value;
  return c !== null && Boolean(c.turnstile_sitekey) && !c.has_turnstile_secret;
});

onMounted(load);
</script>

<template>
  <div class="sec-cfg">
    <header class="head">
      <h1>Seguridad — configuración</h1>
      <span class="badge" :class="config?.has_turnstile_secret ? 'on' : 'off'">
        {{ config?.has_turnstile_secret ? 'TURNSTILE ACTIVO' : 'TURNSTILE DESACTIVADO' }}
      </span>
    </header>
    <p class="subtitle">
      Cloudflare Turnstile protege el formulario de inicio de sesión contra bots.
      Cuando la <em>secret</em> está vacía, el widget no se muestra y el login
      funciona sin verificación (modo desarrollo).
    </p>
    <div v-if="isHalfSet" class="warn">
      Tiene una <strong>sitekey</strong> guardada pero la <strong>secret</strong> está vacía.
      En esta configuración el widget queda desactivado: el frontend no lo renderiza y el backend
      no verifica. Pegue la secret correspondiente o borre la sitekey para evitar confusión.
    </div>

    <Card title="Cloudflare Turnstile" :elevation="1">
      <div class="grid">
        <label class="field">
          <span>Sitekey (público)</span>
          <input
            type="text"
            v-model="draft.turnstile_sitekey"
            placeholder="0xAAAAAAAAAAAAAAAAAAAAA"
            autocomplete="off"
          />
          <small class="hint inline">
            Visible para el cliente. La sitekey se obtiene en
            <a href="https://dash.cloudflare.com/" target="_blank" rel="noopener">dashboard de Cloudflare → Turnstile</a>.
          </small>
        </label>
        <div class="field key-field">
          <span class="label-row">
            <span>Secret (encriptada en SQLite)</span>
            <code class="masked" v-if="config?.has_turnstile_secret">
              {{ config?.turnstile_secret_tag || '••••••' }}
            </code>
            <code class="masked" v-else>no configurada</code>
          </span>
          <div class="key-input">
            <input
              :type="showSecret ? 'text' : 'password'"
              v-model="draft.turnstile_secret"
              placeholder="(pegar nueva secret)"
              autocomplete="off"
            />
            <button type="button" @click="showSecret = !showSecret">
              {{ showSecret ? '🙈' : '👁' }}
            </button>
          </div>
          <small class="hint inline">
            Deje el campo vacío para conservar la secret persistida. Para
            desactivar el widget, use "Borrar secret".
          </small>
        </div>
      </div>
    </Card>

    <div class="actions">
      <button class="btn primary" :disabled="saving || loading" @click="save">
        {{ saving ? 'Guardando…' : 'Guardar' }}
      </button>
      <button
        v-if="config?.has_turnstile_secret"
        class="btn danger"
        :disabled="saving || loading"
        @click="clearSecret"
      >
        Borrar secret (desactivar widget)
      </button>
      <button class="btn ghost" :disabled="loading" @click="load">↻ Refrescar</button>
    </div>

    <Card title="Reglas anti-abuso ya activas" :elevation="0">
      <ul class="bullets">
        <li>Rate limit por IP en <code>POST /api/auth/login</code> (5 intentos / 15 min).</li>
        <li>Rate limit por sesión en el tutor (20 preguntas / hora).</li>
        <li>Token bucket global del LLM configurable en <code>/admin/tutor/config</code>.</li>
        <li>El panel del tutor exige usuario autenticado.</li>
      </ul>
    </Card>
  </div>
</template>

<style scoped>
.sec-cfg { display: flex; flex-direction: column; gap: var(--space-4); }
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
.subtitle { font-size: 12px; color: var(--text-dim); margin: 0; line-height: 1.5; max-width: 60ch; }
.subtitle em { color: var(--amber); font-style: normal; }
.subtitle a { color: #93c5fd; }

.warn {
  background: rgba(239, 68, 68, 0.08);
  border: 1px solid rgba(239, 68, 68, 0.4);
  color: #fca5a5;
  padding: 0.55rem 0.85rem;
  border-radius: 0.35rem;
  font-size: 12px;
  line-height: 1.5;
  max-width: 60ch;
}
.warn strong { color: #fee2e2; }

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 0.85rem;
}
.field { display: flex; flex-direction: column; gap: 0.3rem; font-size: 12px; }
.field > span:not(.label-row) { color: var(--text-dim); letter-spacing: 0.5px; text-transform: uppercase; font-size: 10px; }
.field input {
  background: var(--surface-overlay);
  border: 1px solid rgba(245, 158, 11, 0.3);
  color: var(--text);
  padding: 0.45rem 0.55rem;
  font-family: inherit;
  font-size: 13px;
  border-radius: 0.25rem;
  outline: none;
}
.field input:focus { border-color: var(--amber); }
.field small.inline { font-size: 10px; color: var(--text-dim); line-height: 1.45; }
.field small.inline a { color: #93c5fd; }

.label-row { display: flex; justify-content: space-between; align-items: baseline; gap: 0.5rem; }
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
.btn.primary { background: rgba(245, 158, 11, 0.18); border-color: var(--amber); color: var(--amber); }
.btn.primary:hover:not(:disabled) { background: rgba(245, 158, 11, 0.3); }
.btn.danger { background: rgba(239, 68, 68, 0.12); border-color: #ef4444; color: #fca5a5; }
.btn.danger:hover:not(:disabled) { background: rgba(239, 68, 68, 0.25); }
.btn.ghost { background: transparent; border-color: rgba(245, 158, 11, 0.3); color: var(--text); }
.btn.ghost:hover:not(:disabled) { background: rgba(245, 158, 11, 0.06); }
.btn:disabled { opacity: 0.45; cursor: not-allowed; }

.bullets { margin: 0; padding-left: 1.2rem; color: var(--text-dim); font-size: 12px; line-height: 1.6; }
.bullets code { background: rgba(245, 158, 11, 0.08); color: var(--amber); padding: 0 0.3rem; border-radius: 0.2rem; font-family: 'JetBrains Mono', monospace; font-size: 11px; }
</style>
