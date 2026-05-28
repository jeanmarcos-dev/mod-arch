<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { Copy, Share2, X, Check } from '@lucide/vue';
import Card from '../../components/ui/Card.vue';
import Btn from '../../components/ui/Btn.vue';
import Field from '../../components/ui/Field.vue';
import { useAuthStore } from '../../stores/auth';
import { useCpuStore } from '../../stores/cpu';
import { useToastStore } from '../../stores/toast';

const { t } = useI18n();
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
    toast.err(t('profile.mySim.copyError'));
  }
}
</script>

<template>
  <div class="profile">
    <Card :title="t('profile.title')" :elevation="1" class="profile__card">
      <div v-if="hasUser" class="profile__body">
        <Field :label="t('profile.fields.name')" v-slot="{ id }">
          <input :id="id" :value="auth.user?.name" readonly />
        </Field>
        <Field
          v-if="auth.user?.email"
          :label="t('profile.fields.email')"
          :help="t('profile.fields.emailHelp')"
          v-slot="{ id }"
        >
          <input :id="id" :value="auth.user.email" readonly />
        </Field>
        <Field :label="t('profile.fields.internalId')" v-slot="{ id }">
          <input :id="id" :value="auth.user?.id" readonly />
        </Field>

        <div class="profile__actions">
          <Btn variant="danger" @click="logout">{{ t('profile.logout') }}</Btn>
        </div>
      </div>

      <div v-else class="profile__empty">
        <p>{{ t('profile.emptyHint') }}</p>
        <Btn @click="router.push('/login')">{{ t('profile.identify') }}</Btn>
      </div>
    </Card>

    <Card v-if="hasUser && isOwner" :title="t('profile.mySim.title')" :elevation="1" class="profile__card">
      <p class="profile__hint">{{ t('profile.mySim.hint') }}</p>

      <Field
        :label="t('profile.mySim.aliasLabel')"
        :help="t('profile.mySim.aliasHelp')"
        v-slot="{ id }"
      >
        <div class="profile__alias-row">
          <input
            :id="id"
            v-model="aliasDraft"
            maxlength="32"
            :placeholder="t('profile.mySim.aliasPlaceholder')"
            @keydown.enter.prevent="saveAlias"
          />
          <Btn
            size="sm"
            :disabled="!aliasDirty || aliasDraft.trim() === ''"
            @click="saveAlias"
          >
            <Check v-if="aliasJustSaved" :size="13" :stroke-width="2" />
            <span>{{ aliasJustSaved ? t('profile.mySim.saved') : t('profile.mySim.save') }}</span>
          </Btn>
        </div>
      </Field>

      <div class="profile__share">
        <div class="profile__share-row">
          <div>
            <strong class="profile__share-title">
              {{ isShared ? t('profile.mySim.sharedTitle') : t('profile.mySim.privateTitle') }}
            </strong>
            <p class="profile__share-desc">
              {{ isShared ? t('profile.mySim.sharedDesc') : t('profile.mySim.privateDesc') }}
            </p>
          </div>
          <Btn :variant="isShared ? 'danger' : 'primary'" size="sm" @click="toggleShare">
            <X v-if="isShared" :size="13" :stroke-width="2" />
            <Share2 v-else :size="13" :stroke-width="2" />
            <span>{{ isShared ? t('profile.mySim.stopSharing') : t('profile.mySim.share') }}</span>
          </Btn>
        </div>

        <div v-if="isShared" class="profile__share-url">
          <input :value="cpu.viewerLink()" readonly />
          <Btn size="sm" variant="ghost" @click="copyViewURL">
            <Check v-if="copyConfirm" :size="13" :stroke-width="2" />
            <Copy v-else :size="13" :stroke-width="2" />
            <span>{{ copyConfirm ? t('profile.mySim.copied') : t('profile.mySim.copy') }}</span>
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
