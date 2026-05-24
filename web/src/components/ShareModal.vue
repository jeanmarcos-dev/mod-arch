<script setup lang="ts">
import { ref, computed } from 'vue';
import { Share2, X, Zap, Copy, Check, Square } from '@lucide/vue';
import { useCpuStore } from '../stores/cpu';

const cpu = useCpuStore();
const props = defineProps<{ open: boolean }>();
const emit = defineEmits<{ (e: 'close'): void }>();

const copied = ref(false);

const link = computed(() => cpu.viewerLink());
const isShared = computed(() => !!cpu.sessionInfo.view_token);

async function copyLink() {
  if (!link.value) return;
  try {
    await navigator.clipboard.writeText(link.value);
    copied.value = true;
    setTimeout(() => { copied.value = false; }, 1500);
  } catch {
    // Fallback: select + execCommand para browsers viejos
    const t = document.createElement('textarea');
    t.value = link.value;
    document.body.appendChild(t);
    t.select();
    try { document.execCommand('copy'); copied.value = true; } catch { /* ignore */ }
    document.body.removeChild(t);
    setTimeout(() => { copied.value = false; }, 1500);
  }
}

function activate() {
  cpu.share();
}

function deactivate() {
  if (confirm('¿Desea detener el compartido? Los espectadores actuales perderán la conexión.')) {
    cpu.unshare();
  }
}
</script>

<template>
  <Teleport to="body">
    <div v-if="props.open" class="modal-overlay" @click.self="emit('close')">
      <div class="modal" role="dialog" aria-modal="true">
        <header class="modal-head">
          <span class="modal-title"><Share2 :size="14" :stroke-width="2" /> COMPARTIR SESIÓN</span>
          <button class="close-btn" @click="emit('close')" title="Cerrar (Esc)" aria-label="Cerrar"><X :size="16" :stroke-width="2" /></button>
        </header>

        <div class="modal-body">
          <p class="lead">
            Genere un enlace <strong>solo lectura</strong> que cualquier persona
            puede abrir para ver su sesión en tiempo real. Los espectadores verán
            todo lo que usted hace, pero no podrán tocar nada.
          </p>

          <template v-if="!isShared">
            <button class="cta primary" @click="activate">
              <Zap :size="14" :stroke-width="2" />
              <span>Activar compartido</span>
            </button>
            <p class="muted">Se generará un enlace único. Puede desactivarlo cuando lo desee.</p>
          </template>

          <template v-else>
            <label class="field-label">Enlace para compartir</label>
            <div class="link-row">
              <input class="link-input" name="share_link" aria-label="Enlace de la sesión compartida" readonly :value="link" @click="($event.target as HTMLInputElement).select()" />
              <button class="copy-btn" @click="copyLink">
                <component :is="copied ? Check : Copy" :size="14" :stroke-width="2" />
                <span>{{ copied ? 'COPIADO' : 'COPIAR' }}</span>
              </button>
            </div>
            <p class="muted">
              Cualquier persona con este enlace entra como espectador. Si lo
              desactiva, los espectadores conectados quedan sin poder
              reconectarse.
            </p>

            <div class="actions-row">
              <button class="cta danger" @click="deactivate">
                <Square :size="14" :stroke-width="2" />
                <span>Detener compartido</span>
              </button>
            </div>
          </template>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.75);
  z-index: 2000;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  backdrop-filter: blur(2px);
}

.modal {
  background: var(--bg-1);
  border: 1px solid var(--border-strong);
  box-shadow: 0 16px 48px rgba(0, 0, 0, 0.8), 0 0 24px rgba(var(--accent-rgb), 0.15);
  width: 100%;
  max-width: 520px;
  font-family: 'JetBrains Mono', monospace;
}

.modal-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  border-bottom: 1px solid var(--border);
  background: rgba(var(--accent-rgb), 0.05);
}

.modal-title {
  font-family: var(--font-display);
  font-size: 13px;
  letter-spacing: 3px;
  color: var(--amber);
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.close-btn {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-dim);
  width: 28px;
  height: 28px;
  /* Reset global button defaults that crowd out the SVG: padding eats the
     16px icon inside a 28px box, and min-height bloats the height. */
  padding: 0;
  min-height: 0;
  cursor: pointer;
  font-size: 14px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-sm);
  flex-shrink: 0;
}

.close-btn svg { display: block; }

.close-btn:hover {
  color: var(--red-soft);
  border-color: var(--red-soft);
}

.modal-body {
  padding: 20px 16px 16px;
}

.lead {
  color: var(--text-dim);
  font-size: 13px;
  line-height: 1.6;
  margin-bottom: 16px;
}

.lead strong { color: var(--amber); }

.field-label {
  display: block;
  font-size: 11px;
  color: var(--text-dim);
  letter-spacing: 2px;
  text-transform: uppercase;
  margin-bottom: 6px;
}

.link-row {
  display: flex;
  gap: 6px;
  margin-bottom: 8px;
}

.link-input {
  flex: 1;
  background: var(--surface-overlay);
  border: 1px solid var(--border);
  color: var(--cyan);
  padding: 8px 10px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  min-width: 0;
}

.link-input:focus {
  outline: none;
  border-color: var(--cyan);
}

.copy-btn {
  background: rgba(106, 212, 255, 0.1);
  border: 1px solid var(--cyan);
  color: var(--cyan);
  padding: 8px 14px;
  font-family: inherit;
  font-size: 11px;
  letter-spacing: 2px;
  cursor: pointer;
  white-space: nowrap;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.copy-btn:hover {
  background: rgba(106, 212, 255, 0.25);
}

.cta {
  background: rgba(var(--accent-rgb), 0.1);
  border: 1px solid var(--amber);
  color: var(--amber);
  padding: 12px 24px;
  font-family: inherit;
  font-size: 13px;
  letter-spacing: 2px;
  text-transform: uppercase;
  cursor: pointer;
  width: 100%;
  margin-bottom: 6px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
}

.cta:hover {
  background: rgba(var(--accent-rgb), 0.2);
  box-shadow: 0 0 8px rgba(var(--accent-rgb), 0.3);
}

.cta.danger {
  color: var(--red-soft);
  border-color: var(--red-soft);
  background: rgba(255, 122, 122, 0.06);
}

.cta.danger:hover {
  background: rgba(255, 122, 122, 0.15);
  box-shadow: 0 0 8px rgba(255, 122, 122, 0.3);
}

.muted {
  color: var(--text-muted);
  font-size: 11px;
  font-style: italic;
  margin-top: 4px;
  line-height: 1.4;
}

.actions-row {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px dashed var(--border);
}

@media (max-width: 768px) {
  .modal-overlay { padding: 0; align-items: flex-start; }
  .modal {
    max-width: none;
    min-height: 100vh;
    border: none;
    border-top: 2px solid var(--amber);
  }
  .modal-body { padding: var(--space-3); }
  .link-row { flex-wrap: wrap; }
  .copy-btn { flex: 1 1 100%; justify-content: center; }
}
</style>
