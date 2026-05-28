<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { Users, ShieldCheck, ShieldUser, GraduationCap, UserPlus, Pencil, Trash2, Lock } from '@lucide/vue';
import { useAdminStore, type UserRow, type UserRole, ENV_ADMIN_USER_ID } from '../../stores/admin';
import { useToastStore } from '../../stores/toast';
import Card from '../../components/ui/Card.vue';
import Btn from '../../components/ui/Btn.vue';
import DataTable, { type Column } from '../../components/ui/DataTable.vue';
import UserFormModal from '../../components/admin/UserFormModal.vue';
import { formatTimestamp } from './util';

const { t } = useI18n();
const admin = useAdminStore();
const toast = useToastStore();

const columns = computed<Column<UserRow>[]>(() => [
  { id: 'name', label: t('admin.users.colName') },
  { id: 'email', label: t('admin.users.colEmail'), accessor: (u) => u.email || '' },
  { id: 'student_id', label: t('admin.users.colStudentId'), accessor: (u) => u.student_id || '' },
  { id: 'role', label: t('admin.users.colRole'), accessor: (u) => u.role, width: '140px' },
  { id: 'last_login_at', label: t('admin.users.colLastLogin'), accessor: (u) => u.last_login_at, width: '170px' },
  { id: 'sessions', label: t('admin.users.colSessions'), accessor: (u) => u.sessions.length, align: 'right' },
]);

const roleLabel = computed<Record<UserRole, string>>(() => ({
  admin: t('admin.users.roleAdmin'),
  moderator: t('admin.users.roleModerator'),
  student: t('admin.users.roleStudent'),
}));

function roleIcon(r: UserRole) {
  if (r === 'admin') return ShieldCheck;
  if (r === 'moderator') return ShieldUser;
  return GraduationCap;
}

// Drives the inline role picker. Only one row stays open at a time so users
// cannot accidentally fire two role updates back to back.
const openRoleMenu = ref<string | null>(null);

function toggleRoleMenu(id: string) {
  openRoleMenu.value = openRoleMenu.value === id ? null : id;
}

// canChangeRoleFor encodes the two non-DB rules: the env admin is irrevocable,
// and admins cannot demote themselves (prevents accidental lock-out when an
// org has a single admin).
function canChangeRoleFor(row: UserRow): boolean {
  if (!admin.canChangeRoles) return false;
  if (row.id === ENV_ADMIN_USER_ID) return false;
  if (row.id === admin.userId) return false;
  return true;
}

async function kickUser(id: string, name: string) {
  if (!confirm(t('admin.users.confirmKick', { name }))) return;
  try {
    await admin.kickUser(id);
    toast.ok(t('admin.users.okKick'));
  } catch (e) {
    toast.err(e instanceof Error ? e.message : t('admin.sessions.errorGeneric'));
  }
}

async function changeRole(row: UserRow, next: UserRole) {
  openRoleMenu.value = null;
  if (row.role === next) return;
  const verb = next === 'admin'
    ? t('admin.users.verbPromoteAdmin')
    : next === 'moderator'
      ? t('admin.users.verbPromoteModerator')
      : t('admin.users.verbDemoteStudent');
  if (!confirm(t('admin.users.confirmRoleChange', { verb, name: row.name }))) return;
  try {
    await admin.setUserRole(row.id, next);
    toast.ok(t('admin.users.okRoleUpdated', { role: roleLabel.value[next] }));
  } catch (e) {
    toast.err(e instanceof Error ? e.message : t('admin.sessions.errorGeneric'));
  }
}

// Identity CRUD wiring. The form modal handles its own validation; errors
// surfaced by the server (duplicates, mostly) are forwarded back to the modal
// via the exposed showError helper so the dialog stays open with the message.
const createOpen = ref(false);
const editTarget = ref<UserRow | null>(null);
const createModalRef = ref<InstanceType<typeof UserFormModal> | null>(null);
const editModalRef = ref<InstanceType<typeof UserFormModal> | null>(null);

// canEdit / canDelete encode the env-admin protection plus the self-delete
// guard. Editing your own row is allowed; deleting it is not (server enforces
// this anyway, but the disabled button avoids a roundtrip).
function canEdit(row: UserRow): boolean {
  if (!admin.canChangeRoles) return false;
  if (row.id === ENV_ADMIN_USER_ID) return false;
  return true;
}

function canDelete(row: UserRow): boolean {
  if (!admin.canChangeRoles) return false;
  if (row.id === ENV_ADMIN_USER_ID) return false;
  if (row.id === admin.userId) return false;
  return true;
}

async function onCreateSubmit(payload: { name: string; email: string; student_id: string; role: UserRole }) {
  try {
    await admin.createUser(payload);
    toast.ok(t('admin.users.okCreated'));
    createOpen.value = false;
  } catch (e) {
    createModalRef.value?.showError(e instanceof Error ? e.message : t('admin.users.errorCreate'));
  }
}

async function onEditSubmit(payload: { name: string; email: string; student_id: string }) {
  const target = editTarget.value;
  if (!target) return;
  try {
    // Send only the fields that actually changed so the audit log records
    // the minimal diff instead of every field on every save.
    const patch: Record<string, string> = {};
    if (payload.name !== target.name) patch.name = payload.name;
    if (payload.email !== (target.email ?? '')) patch.email = payload.email;
    if (payload.student_id !== (target.student_id ?? '')) patch.student_id = payload.student_id;
    if (Object.keys(patch).length === 0) {
      editTarget.value = null;
      return;
    }
    await admin.updateUser(target.id, patch);
    toast.ok(t('admin.users.okUpdated'));
    editTarget.value = null;
  } catch (e) {
    editModalRef.value?.showError(e instanceof Error ? e.message : t('admin.users.errorUpdate'));
  }
}

async function deleteUser(row: UserRow) {
  const sessionsNote = row.sessions.length
    ? t('admin.users.sessionsNote', { count: row.sessions.length })
    : '';
  if (!confirm(t('admin.users.confirmDeleteBase', { name: row.name, sessionsNote }))) {
    return;
  }
  try {
    await admin.deleteUser(row.id);
    toast.ok(t('admin.users.okDeleted'));
  } catch (e) {
    toast.err(e instanceof Error ? e.message : t('admin.users.errorDelete'));
  }
}
</script>

<template>
  <Card :title="t('admin.users.title')">
    <template v-if="admin.canChangeRoles" #header>
      <Btn variant="primary" size="sm" @click="createOpen = true">
        <UserPlus :size="12" :stroke-width="2" />
        <span>{{ t('admin.users.newUser') }}</span>
      </Btn>
    </template>
    <DataTable
      :columns="columns"
      :rows="admin.users"
      initial-sort="last_login_at"
      :search-placeholder="t('admin.users.searchPlaceholder')"
      :empty-icon="Users"
      :empty-title="t('admin.users.emptyTitle')"
      :empty-message="t('admin.users.emptyMessage')"
    >
      <template #[`cell:name`]="{ row }">
        <span class="name-cell">
          <span>{{ row.name }}</span>
          <span
            v-if="row.id === ENV_ADMIN_USER_ID"
            class="env-badge"
            :title="t('admin.users.envBadgeTitle')"
          >
            <Lock :size="10" :stroke-width="2" />
            <span>env</span>
          </span>
        </span>
      </template>
      <template #[`cell:email`]="{ value }">
        <span class="dim">{{ value || '—' }}</span>
      </template>
      <template #[`cell:student_id`]="{ value }">
        <span class="dim">{{ value || '—' }}</span>
      </template>
      <template #[`cell:role`]="{ row }">
        <span :class="['role-chip', `role-chip--${row.role}`]" :title="roleLabel[row.role]">
          <component :is="roleIcon(row.role)" :size="12" :stroke-width="2" />
          <span>{{ roleLabel[row.role] }}</span>
        </span>
      </template>
      <template #[`cell:last_login_at`]="{ row }">
        <span class="dim mono">{{ formatTimestamp(row.last_login_at) }}</span>
      </template>
      <template #[`cell:sessions`]="{ row }">
        <span v-if="row.sessions.length" class="pill">{{ row.sessions.length }}</span>
        <span v-else class="dim">—</span>
      </template>
      <template #actions="{ row }">
        <div class="actions">
          <div class="role-action" v-if="canChangeRoleFor(row)">
            <Btn
              variant="ghost"
              size="sm"
              :aria-expanded="openRoleMenu === row.id"
              @click="toggleRoleMenu(row.id)"
            >{{ t('admin.users.actionRole') }}</Btn>
            <ul v-if="openRoleMenu === row.id" class="role-menu" role="menu">
              <li v-for="r in (['student', 'moderator', 'admin'] as UserRole[])" :key="r">
                <button
                  type="button"
                  class="role-menu__item"
                  :class="{ 'role-menu__item--active': row.role === r }"
                  :disabled="row.role === r"
                  role="menuitem"
                  @click="changeRole(row, r)"
                >
                  <component :is="roleIcon(r)" :size="12" :stroke-width="2" />
                  <span>{{ roleLabel[r] }}</span>
                </button>
              </li>
            </ul>
          </div>
          <Btn
            variant="ghost"
            size="sm"
            :disabled="!canEdit(row)"
            :title="t('admin.users.actionEditHint')"
            @click="editTarget = row"
          >
            <Pencil :size="12" :stroke-width="2" />
            <span>{{ t('admin.users.actionEdit') }}</span>
          </Btn>
          <Btn
            variant="danger"
            size="sm"
            :disabled="!row.sessions.length"
            @click="kickUser(row.id, row.name)"
          >{{ t('admin.users.actionKick') }}</Btn>
          <Btn
            variant="danger"
            size="sm"
            :disabled="!canDelete(row)"
            :title="t('admin.users.actionDeleteHint')"
            @click="deleteUser(row)"
          >
            <Trash2 :size="12" :stroke-width="2" />
          </Btn>
        </div>
      </template>
    </DataTable>
  </Card>

  <UserFormModal
    ref="createModalRef"
    :open="createOpen"
    mode="create"
    @close="createOpen = false"
    @submit="onCreateSubmit"
  />
  <UserFormModal
    ref="editModalRef"
    :open="editTarget !== null"
    mode="edit"
    :initial="editTarget"
    @close="editTarget = null"
    @submit="onEditSubmit"
  />
</template>

<style scoped>
.dim { color: var(--text-dim); }
.mono { font-family: 'JetBrains Mono', monospace; font-size: 11px; }

.pill {
  font-size: 10px;
  padding: 2px 8px;
  border: 1px solid var(--border-strong);
  color: var(--amber);
  letter-spacing: 1px;
  border-radius: var(--radius-sm);
  background: rgba(var(--accent-rgb), 0.08);
}

/* Role chip mirrors the pill style but switches color by role so the column
   reads as a glance. Admins inherit the theme accent; moderators get the
   cyan info color; students stay muted on purpose. */
.role-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 10px;
  letter-spacing: 1px;
  text-transform: uppercase;
  padding: 2px 8px;
  border: 1px solid currentColor;
  border-radius: var(--radius-sm);
}
.role-chip--admin     { color: var(--accent); background: rgba(var(--accent-rgb), 0.08); }
.role-chip--moderator { color: var(--cyan); background: rgba(106, 212, 255, 0.08); }
.role-chip--student   { color: var(--text-muted); }

.actions {
  display: flex;
  align-items: center;
  gap: var(--space-1);
}

.role-action { position: relative; }

.role-menu {
  position: absolute;
  top: calc(100% + 4px);
  right: 0;
  list-style: none;
  margin: 0;
  padding: 4px;
  background: var(--surface-2);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-sm);
  box-shadow: var(--shadow-elevation-2);
  z-index: var(--z-dropdown);
  min-width: 180px;
}

.role-menu__item {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  width: 100%;
  padding: 6px 10px;
  background: transparent;
  border: 1px solid transparent;
  color: var(--text);
  font-size: 12px;
  font-family: inherit;
  text-transform: none;
  letter-spacing: 0;
  cursor: pointer;
  border-radius: var(--radius-sm);
  text-align: left;
  transition: background var(--transition-fast), color var(--transition-fast);
}
.role-menu__item:hover:not(:disabled) {
  background: rgba(var(--accent-rgb), 0.10);
  color: var(--accent);
}
.role-menu__item--active {
  color: var(--accent);
  opacity: 0.7;
  cursor: default;
}
.role-menu__item:disabled { cursor: not-allowed; }

/* Visual cue for the synthetic env-admin row: a small inline badge next to
   the name so the operator can tell at a glance which entry is the
   irrevocable env-configured account. */
.name-cell {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
}

.env-badge {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 9px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  color: var(--amber);
  border: 1px solid var(--amber);
  background: rgba(var(--accent-rgb), 0.08);
  padding: 1px 5px;
  border-radius: var(--radius-sm);
}
</style>
