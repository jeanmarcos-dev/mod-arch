<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { Copy, Share2, X, Check } from '@lucide/vue';
import Card from '../../components/ui/Card.vue';
import Btn from '../../components/ui/Btn.vue';
import Field from '../../components/ui/Field.vue';
import { useAuthStore } from '../../stores/auth';
import { useCpuStore } from '../../stores/cpu';
import { useToastStore } from '../../stores/toast';

const auth = useAuthStore();
const cpu = useCpuStore();
const toast = useToastStore();
const router = useRouter();

const hasUser = computed(() => Boolean(auth.user));

// Local alias state shadows sessionInfo.label so we can render an inline
// editor without losing the user's keystrokes when a WebSocket update lands.
const aliasDraft = ref('');
const aliasJustSaved = ref(false);
const copyConfirm = ref(false);

watch(
  () => cpu.sessionInfo.label,
  (val) => { aliasDraft.value = val ?? ''; },
  { immediate: true },
);

const aliasDirty = computed(
  () => aliasDraft.value.trim() !== (cpu.sessionInfo.label ?? '').trim(),
);
const isShared = computed(() => Boolean(cpu.sessionInfo.view_token));
const isOwner = computed(() => cpu.sessionInfo.role === 'owner');

onMounted(() => {
  if (!auth.user) auth.fetchMe();
});

async function logout() {
  await auth.logout();
  router.replace('/login');
}

function saveAlias() {
  const next = aliasDraft.value.trim();
  if (!next || !aliasDirty.value) return;
  cpu.setAlias(next);
  aliasJustSaved.value = true;
  setTimeout(() => { aliasJustSaved.value = false; }, 1500);
}

function toggleShare() {
  if (isShared.value) cpu.unshare();
  else cpu.share();
}

async function copyViewURL() {
  const url = cpu.viewerLink();
  if (!url) return;
  try {
    await navigator.clipboard.writeText(url);
    copyConfirm.value = true;
    setTimeout(() => { copyConfirm.value = false; }, 1500);
  } catch {
    toast.err('No se pudo copiar al portapapeles');
  }
}
</script>

<template>
  <div class="profile">
    <Card title="Perfil" :elevation="1" class="profile__card">
      <div v-if="hasUser" class="profile__body">
        <Field label="Nombre" v-slot="{ id }">
          <input :id="id" :value="auth.user?.name" readonly />
        </Field>
        <Field
          v-if="auth.user?.email"
          label="Correo"
          help="Use el correo para volver a entrar desde otro dispositivo via enlace mágico."
          v-slot="{ id }"
        >
          <input :id="id" :value="auth.user.email" readonly />
        </Field>
        <Field label="ID interno" v-slot="{ id }">
          <input :id="id" :value="auth.user?.id" readonly />
        </Field>

        <div class="profile__actions">
          <Btn variant="danger" @click="logout">Cerrar sesión</Btn>
        </div>
      </div>

      <div v-else class="profile__empty">
        <p>
          No ha iniciado sesión. Identifíquese para que su simulador quede
          asociado a su nombre y pueda recuperarlo desde otro dispositivo con
          el enlace mágico.
        </p>
        <Btn @click="router.push('/login')">Identificarse</Btn>
      </div>
    </Card>

    <Card v-if="hasUser && isOwner" title="Mi simulador" :elevation="1" class="profile__card">
      <p class="profile__hint">
        Cada usuario identificado tiene un único simulador. Aquí puede
        cambiarle el alias visible (lo verán quienes se conecten como
        espectadores) y compartirlo con un enlace de solo lectura.
      </p>

      <Field
        label="Alias visible"
        help="Aparece en la barra superior del simulador y en las pantallas de los espectadores."
        v-slot="{ id }"
      >
        <div class="profile__alias-row">
          <input
            :id="id"
            v-model="aliasDraft"
            maxlength="32"
            placeholder="Sin alias asignado"
            @keydown.enter.prevent="saveAlias"
          />
          <Btn
            size="sm"
            :disabled="!aliasDirty || aliasDraft.trim() === ''"
            @click="saveAlias"
          >
            <Check v-if="aliasJustSaved" :size="13" :stroke-width="2" />
            <span>{{ aliasJustSaved ? 'Guardado' : 'Guardar' }}</span>
          </Btn>
        </div>
      </Field>

      <div class="profile__share">
        <div class="profile__share-row">
          <div>
            <strong class="profile__share-title">
              {{ isShared ? 'Simulador compartido' : 'Simulador privado' }}
            </strong>
            <p class="profile__share-desc">
              {{ isShared
                ? 'Cualquiera con el enlace puede ver su simulador en modo solo lectura.'
                : 'Solo usted ve este simulador. Active el modo compartido para generar un enlace.'
              }}
            </p>
          </div>
          <Btn :variant="isShared ? 'danger' : 'primary'" size="sm" @click="toggleShare">
            <X v-if="isShared" :size="13" :stroke-width="2" />
            <Share2 v-else :size="13" :stroke-width="2" />
            <span>{{ isShared ? 'Dejar de compartir' : 'Compartir' }}</span>
          </Btn>
        </div>

        <div v-if="isShared" class="profile__share-url">
          <input :value="cpu.viewerLink()" readonly />
          <Btn size="sm" variant="ghost" @click="copyViewURL">
            <Check v-if="copyConfirm" :size="13" :stroke-width="2" />
            <Copy v-else :size="13" :stroke-width="2" />
            <span>{{ copyConfirm ? 'Copiado' : 'Copiar' }}</span>
          </Btn>
        </div>
      </div>
    </Card>
  </div>
</template>

<style scoped>
.profile {
  flex: 1;
  padding: var(--space-5);
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.profile__card {
  max-width: 560px;
  width: 100%;
  margin: 0 auto;
}

.profile__body,
.profile__empty {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.profile__empty p {
  color: var(--text-dim);
  font-size: 13px;
  line-height: 1.6;
}

.profile__actions {
  margin-top: var(--space-2);
}

.profile__hint {
  color: var(--text-dim);
  font-size: 12px;
  line-height: 1.5;
  margin-bottom: var(--space-3);
}

.profile__alias-row {
  display: flex;
  gap: var(--space-2);
  align-items: center;
}
.profile__alias-row input {
  flex: 1;
}

.profile__share {
  margin-top: var(--space-3);
  padding-top: var(--space-3);
  border-top: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.profile__share-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3);
}

.profile__share-title {
  color: var(--accent);
  font-family: var(--font-display);
  font-size: 12px;
  letter-spacing: 1.5px;
}

.profile__share-desc {
  margin: var(--space-1) 0 0;
  color: var(--text-dim);
  font-size: 12px;
  line-height: 1.5;
}

.profile__share-url {
  display: flex;
  gap: var(--space-2);
  align-items: center;
}
.profile__share-url input {
  flex: 1;
  font-family: var(--font-code);
  font-size: 12px;
}
</style>
