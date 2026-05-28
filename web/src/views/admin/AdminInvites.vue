<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { Mail } from '@lucide/vue';
import { useAdminStore } from '../../stores/admin';
import { useToastStore } from '../../stores/toast';
import Card from '../../components/ui/Card.vue';
import Btn from '../../components/ui/Btn.vue';
import Field from '../../components/ui/Field.vue';
import EmptyState from '../../components/ui/EmptyState.vue';
import { formatAgo } from './util';

const { t } = useI18n();
const admin = useAdminStore();
const toast = useToastStore();

const inviteLabel = ref('');
const busy = ref(false);

async function createInvite() {
  const label = inviteLabel.value.trim();
  busy.value = true;
  try {
    const inv = await admin.createInvite(label);
    inviteLabel.value = '';
    try {
      await navigator.clipboard.writeText(inv.url);
      toast.ok(t('admin.invites.okCreatedCopied'));
    } catch {
      toast.ok(t('admin.invites.okCreated', { url: inv.url }));
    }
  } catch (e) {
    toast.err(e instanceof Error ? e.message : t('admin.invites.errorCreate'));
  } finally {
    busy.value = false;
  }
}

async function copyInvite(url: string) {
  try {
    await navigator.clipboard.writeText(url);
    toast.ok(t('admin.invites.okCopied'));
  } catch {
    toast.err(t('admin.invites.copyError'));
  }
}

async function revokeInvite(token: string) {
  if (!confirm(t('admin.invites.confirmRevoke'))) return;
  try {
    await admin.revokeInvite(token);
    toast.ok(t('admin.invites.okRevoked'));
  } catch (e) {
    toast.err(e instanceof Error ? e.message : t('admin.sessions.errorGeneric'));
  }
}
</script>

<template>
  <Card :title="t('admin.invites.title')">
    <form class="invite-form" @submit.prevent="createInvite">
      <Field :label="t('admin.invites.labelField')" v-slot="{ id }">
        <input
          :id="id"
          v-model="inviteLabel"
          name="invite_label"
          :placeholder="t('admin.invites.labelPlaceholder')"
          maxlength="32"
          :disabled="busy"
        />
      </Field>
      <Btn type="submit" :loading="busy">{{ t('admin.invites.submit') }}</Btn>
    </form>

    <table v-if="admin.invites.length" class="grid">
      <thead>
        <tr>
          <th>{{ t('admin.invites.colToken') }}</th>
          <th>{{ t('admin.invites.colLabel') }}</th>
          <th>{{ t('admin.invites.colCreated') }}</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="inv in admin.invites" :key="inv.token">
          <td class="mono">{{ inv.token.slice(0, 10) }}…</td>
          <td>
            <span v-if="inv.label">{{ inv.label }}</span>
            <span v-else class="dim">{{ t('admin.invites.noAlias') }}</span>
          </td>
          <td class="dim">{{ t('admin.invites.ago', { time: formatAgo(inv.created_at) }) }}</td>
          <td class="row-actions">
            <Btn variant="ghost" size="sm" @click="copyInvite(inv.url)">{{ t('admin.invites.actionCopy') }}</Btn>
            <Btn variant="danger" size="sm" @click="revokeInvite(inv.token)">{{ t('admin.invites.actionRevoke') }}</Btn>
          </td>
        </tr>
      </tbody>
    </table>
    <EmptyState
      v-else
      :icon="Mail"
      :title="t('admin.invites.emptyTitle')"
      :message="t('admin.invites.emptyMessage')"
    />
  </Card>
</template>

<style scoped>
.invite-form {
  display: flex;
  align-items: flex-end;
  gap: var(--space-3);
  margin-bottom: var(--space-4);
}

.invite-form > :deep(.field) { flex: 1; min-width: 0; }

.grid {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.grid th, .grid td {
  padding: 9px 12px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  text-align: left;
}

.grid th {
  font-size: 10px;
  letter-spacing: 1.5px;
  color: var(--text-dim);
  text-transform: uppercase;
  font-weight: 500;
}

.grid tbody tr:hover { background: rgba(var(--accent-rgb), 0.04); }

.mono { font-family: 'JetBrains Mono', monospace; font-size: 11px; }
.dim { color: var(--text-dim); }

.row-actions {
  display: flex;
  gap: var(--space-1);
  justify-content: flex-end;
  flex-wrap: wrap;
}
</style>
