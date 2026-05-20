<script setup lang="ts">
import { computed, onMounted } from 'vue';
import { useRouter } from 'vue-router';
import Card from '../../components/ui/Card.vue';
import Btn from '../../components/ui/Btn.vue';
import Field from '../../components/ui/Field.vue';
import { useAuthStore } from '../../stores/auth';

const auth = useAuthStore();
const router = useRouter();

const hasUser = computed(() => Boolean(auth.user));

onMounted(() => {
  if (!auth.user) auth.fetchMe();
});

async function logout() {
  await auth.logout();
  router.replace('/login');
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
          No ha iniciado sesión. Identifíquese para que sus sesiones sean
          recordadas entre visitas y poder reenviarle el enlace mágico.
        </p>
        <Btn @click="router.push('/login')">Identificarse</Btn>
      </div>
    </Card>
  </div>
</template>

<style scoped>
.profile {
  flex: 1;
  padding: var(--space-5);
  overflow-y: auto;
}

.profile__card {
  max-width: 560px;
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
</style>
