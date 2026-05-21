<script setup lang="ts">
import { ref } from 'vue';
import { Users, ShieldCheck, ShieldUser, GraduationCap } from '@lucide/vue';
import { useAdminStore, type UserRow, type UserRole, ENV_ADMIN_USER_ID } from '../../stores/admin';
import { useToastStore } from '../../stores/toast';
import Card from '../../components/ui/Card.vue';
import Btn from '../../components/ui/Btn.vue';
import DataTable, { type Column } from '../../components/ui/DataTable.vue';
import { formatTimestamp } from './util';

const admin = useAdminStore();
const toast = useToastStore();

const columns: Column<UserRow>[] = [
  { id: 'name', label: 'Nombre' },
  { id: 'email', label: 'Correo', accessor: (u) => u.email || '' },
  { id: 'student_id', label: 'ID estudiante', accessor: (u) => u.student_id || '' },
  { id: 'role', label: 'Rol', accessor: (u) => u.role, width: '140px' },
  { id: 'last_login_at', label: 'Último ingreso', accessor: (u) => u.last_login_at, width: '170px' },
  { id: 'sessions', label: 'Activas', accessor: (u) => u.sessions.length, align: 'right' },
];

const ROLE_LABEL: Record<UserRole, string> = {
  admin: 'Administrador',
  moderator: 'Moderador',
  student: 'Estudiante',
};

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
  if (!confirm(`¿Expulsar al usuario ${name}? Sus sesiones activas se cerrarán.`)) return;
  try {
    await admin.kickUser(id);
    toast.ok('Usuario expulsado');
  } catch (e) {
    toast.err(e instanceof Error ? e.message : 'Error');
  }
}

async function changeRole(row: UserRow, next: UserRole) {
  openRoleMenu.value = null;
  if (row.role === next) return;
  const verb = next === 'admin'
    ? 'promover a administrador'
    : next === 'moderator'
      ? 'promover a moderador'
      : 'degradar a estudiante';
  if (!confirm(`¿Confirma ${verb} a ${row.name}?`)) return;
  try {
    await admin.setUserRole(row.id, next);
    toast.ok(`Rol actualizado: ${ROLE_LABEL[next]}`);
  } catch (e) {
    toast.err(e instanceof Error ? e.message : 'Error');
  }
}
</script>

<template>
  <Card title="Usuarios identificados">
    <DataTable
      :columns="columns"
      :rows="admin.users"
      initial-sort="last_login_at"
      search-placeholder="Buscar por nombre, correo o ID estudiante…"
      :empty-icon="Users"
      empty-title="Sin usuarios identificados"
      empty-message="Cuando un alumno se identifique con su nombre y correo aparecerá aquí."
    >
      <template #[`cell:email`]="{ value }">
        <span class="dim">{{ value || '—' }}</span>
      </template>
      <template #[`cell:student_id`]="{ value }">
        <span class="dim">{{ value || '—' }}</span>
      </template>
      <template #[`cell:role`]="{ row }">
        <span :class="['role-chip', `role-chip--${row.role}`]" :title="ROLE_LABEL[row.role]">
          <component :is="roleIcon(row.role)" :size="12" :stroke-width="2" />
          <span>{{ ROLE_LABEL[row.role] }}</span>
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
            >rol…</Btn>
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
                  <span>{{ ROLE_LABEL[r] }}</span>
                </button>
              </li>
            </ul>
          </div>
          <Btn
            variant="danger"
            size="sm"
            :disabled="!row.sessions.length"
            @click="kickUser(row.id, row.name)"
          >expulsar</Btn>
        </div>
      </template>
    </DataTable>
  </Card>
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
</style>
