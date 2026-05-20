<script setup lang="ts">
import { useAdminStore } from '../../stores/admin';
import Card from '../../components/ui/Card.vue';

const admin = useAdminStore();
</script>

<template>
  <div class="overview">
    <h1 class="overview__title">Resumen</h1>
    <p class="overview__subtitle">Conectado como <strong>{{ admin.user }}</strong></p>

    <div v-if="admin.stats" class="overview__stats">
      <Card :surface="2" :padded="false">
        <div class="stat">
          <span class="stat__label">Públicas</span>
          <span class="stat__value">{{ admin.stats.full }} / {{ admin.stats.max_public }}</span>
        </div>
      </Card>
      <Card :surface="2" :padded="false">
        <div class="stat">
          <span class="stat__label">Privilegiadas</span>
          <span class="stat__value">{{ admin.stats.privileged }} / {{ admin.stats.max_privileged }}</span>
        </div>
      </Card>
      <Card :surface="2" :padded="false">
        <div class="stat">
          <span class="stat__label">Creadas (total)</span>
          <span class="stat__value">{{ admin.stats.total_created }}</span>
        </div>
      </Card>
      <Card :surface="2" :padded="false">
        <div class="stat">
          <span class="stat__label">Pico simultáneo</span>
          <span class="stat__value">{{ admin.stats.peak }}</span>
        </div>
      </Card>
    </div>

    <Card title="Atajos">
      <ul class="overview__shortcuts">
        <li><RouterLink to="/admin/sesiones">▶ Ver sesiones activas</RouterLink></li>
        <li><RouterLink to="/admin/invitaciones">✉ Generar invitación privilegiada</RouterLink></li>
        <li><RouterLink to="/admin/accesos">⚿ Cambiar política de acceso</RouterLink></li>
        <li><RouterLink to="/admin/auditoria">☰ Revisar auditoría</RouterLink></li>
      </ul>
    </Card>
  </div>
</template>

<style scoped>
.overview {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.overview__title {
  font-family: 'Major Mono Display', monospace;
  font-size: 18px;
  letter-spacing: 4px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  font-weight: normal;
}

.overview__subtitle {
  font-size: 12px;
  color: var(--text-dim);
  margin-top: calc(-1 * var(--space-3));
}

.overview__subtitle strong {
  color: var(--amber);
  letter-spacing: 1px;
}

.overview__stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: var(--space-3);
}

.stat {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-3) var(--space-4);
}

.stat__label {
  font-size: 10px;
  letter-spacing: 2px;
  text-transform: uppercase;
  color: var(--text-dim);
}

.stat__value {
  font-family: 'JetBrains Mono', monospace;
  font-size: 22px;
  color: var(--amber);
  letter-spacing: 1px;
  text-shadow: var(--shadow-glow-amber);
}

.overview__shortcuts {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  font-size: 13px;
}

.overview__shortcuts a {
  color: var(--text);
  display: inline-block;
  padding: var(--space-1) 0;
  transition: color var(--transition-fast), text-shadow var(--transition-fast);
}

.overview__shortcuts a:hover {
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
}
</style>
