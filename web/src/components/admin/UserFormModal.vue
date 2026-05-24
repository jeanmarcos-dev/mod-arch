<script setup lang="ts">
import { ref, watch, computed } from 'vue';
import { X, UserPlus, Save } from '@lucide/vue';
import Btn from '../ui/Btn.vue';
import Field from '../ui/Field.vue';
import type { UserRow, UserRole } from '../../stores/admin';

type Mode = 'create' | 'edit';

const props = defineProps<{
  open: boolean;
  mode: Mode;
  initial?: UserRow | null;
}>();
const emit = defineEmits<{
  (e: 'close'): void;
  (e: 'submit', payload: { name: string; email: string; student_id: string; role: UserRole }): void;
}>();

const name = ref('');
const email = ref('');
const studentId = ref('');
const role = ref<UserRole>('student');
const serverError = ref<string | null>(null);
const submitting = ref(false);

// Reset the form whenever the modal opens. In edit mode the fields are
// pre-loaded from the row; in create mode they start empty with role=student.
watch(
  () => props.open,
  (isOpen) => {
    if (!isOpen) return;
    serverError.value = null;
    submitting.value = false;
    if (props.mode === 'edit' && props.initial) {
      name.value = props.initial.name;
      email.value = props.initial.email ?? '';
      studentId.value = props.initial.student_id ?? '';
      role.value = props.initial.role;
    } else {
      name.value = '';
      email.value = '';
      studentId.value = '';
      role.value = 'student';
    }
  },
  { immediate: true },
);

const title = computed(() => (props.mode === 'create' ? 'Nuevo usuario' : 'Editar usuario'));
const submitLabel = computed(() => (props.mode === 'create' ? 'Crear' : 'Guardar'));
const submitIcon = computed(() => (props.mode === 'create' ? UserPlus : Save));

const identifierMissing = computed(
  () => props.mode === 'create' && email.value.trim() === '' && studentId.value.trim() === '',
);

// canSubmit gates the primary button. Name is always required; create mode
// also requires at least one identifier so a future magic-link login can find
// the row.
const canSubmit = computed(() => {
  if (submitting.value) return false;
  if (name.value.trim() === '') return false;
  if (identifierMissing.value) return false;
  return true;
});

async function onSubmit() {
  if (!canSubmit.value) return;
  serverError.value = null;
  submitting.value = true;
  try {
    emit('submit', {
      name: name.value.trim(),
      email: email.value.trim(),
      student_id: studentId.value.trim(),
      role: role.value,
    });
  } finally {
    submitting.value = false;
  }
}

// External callers can push a server-side error in (e.g. "correo en uso")
// via this exposed handler so the modal stays open with the message inline.
function showError(msg: string) {
  serverError.value = msg;
  submitting.value = false;
}
defineExpose({ showError });
</script>

<template>
  <Teleport to="body">
    <div v-if="props.open" class="modal-overlay" @click.self="emit('close')">
      <div class="modal" role="dialog" aria-modal="true" :aria-label="title">
        <header class="modal-head">
          <span class="modal-title">
            <component :is="submitIcon" :size="14" :stroke-width="2" />
            <span>{{ title.toUpperCase() }}</span>
          </span>
          <button class="close-btn" @click="emit('close')" title="Cerrar (Esc)" aria-label="Cerrar">
            <X :size="16" :stroke-width="2" />
          </button>
        </header>

        <form class="modal-body" @submit.prevent="onSubmit">
          <Field label="Nombre" required>
            <template #default="{ id }">
              <input
                :id="id"
                v-model="name"
                type="text"
                maxlength="32"
                autocomplete="off"
                placeholder="Ada Lovelace"
                required
              />
            </template>
          </Field>

          <Field
            label="Correo"
            :help="props.mode === 'create' ? 'Cuando el usuario solicite su magic link, se asociará con esta cuenta.' : ''"
          >
            <template #default="{ id }">
              <input
                :id="id"
                v-model="email"
                type="email"
                maxlength="254"
                autocomplete="off"
                placeholder="ada@ucv.ve"
              />
            </template>
          </Field>

          <Field label="ID de estudiante">
            <template #default="{ id }">
              <input
                :id="id"
                v-model="studentId"
                type="text"
                maxlength="32"
                autocomplete="off"
                placeholder="UCV-001"
              />
            </template>
          </Field>

          <Field v-if="props.mode === 'create'" label="Rol" required>
            <template #default="{ id }">
              <select :id="id" v-model="role">
                <option value="student">Estudiante</option>
                <option value="moderator">Moderador</option>
                <option value="admin">Administrador</option>
              </select>
            </template>
          </Field>

          <p v-if="identifierMissing" class="hint">
            Debe indicar al menos un correo o un ID de estudiante.
          </p>
          <p v-if="serverError" class="server-err" role="alert">{{ serverError }}</p>

          <footer class="modal-actions">
            <Btn variant="ghost" type="button" @click="emit('close')">Cancelar</Btn>
            <Btn variant="primary" type="submit" :disabled="!canSubmit" :loading="submitting">
              {{ submitLabel }}
            </Btn>
          </footer>
        </form>
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
  max-width: 480px;
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
  padding: 0;
  min-height: 0;
  cursor: pointer;
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
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.hint {
  font-size: 11px;
  color: var(--amber);
  margin: 0;
}

.server-err {
  font-size: 12px;
  color: var(--red-soft);
  border: 1px solid var(--red-soft);
  background: rgba(255, 122, 122, 0.08);
  padding: 6px 10px;
  border-radius: var(--radius-sm);
  margin: 0;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--space-2);
  padding-top: var(--space-2);
  border-top: 1px dashed var(--border);
  margin-top: var(--space-1);
}
</style>
