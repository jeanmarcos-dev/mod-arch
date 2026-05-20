<script setup lang="ts">
import Card from '../../components/ui/Card.vue';

defineProps<{
  title: string;
  tagline: string;
  /** Bulleted preview of what this section will deliver. */
  preview: string[];
  /** "soon" → not yet implemented, "wip" → in progress, "ready" → live. */
  status?: 'soon' | 'wip' | 'ready';
}>();
</script>

<template>
  <div class="placeholder">
    <Card class="placeholder__card" :elevation="1">
      <header class="placeholder__head">
        <span class="placeholder__status" :class="`placeholder__status--${status ?? 'soon'}`">
          {{ status === 'wip' ? 'En curso' : status === 'ready' ? 'Disponible' : 'Próximamente' }}
        </span>
        <h2 class="placeholder__title">{{ title }}</h2>
        <p class="placeholder__tagline">{{ tagline }}</p>
      </header>

      <section class="placeholder__body">
        <h3 class="placeholder__preview-title">Lo que mostrará esta sección</h3>
        <ul class="placeholder__list">
          <li v-for="(line, idx) in preview" :key="idx">{{ line }}</li>
        </ul>
      </section>

      <footer class="placeholder__foot">
        <p>
          La simulación general continúa corriendo en segundo plano. Vuelva a
          <RouterLink to="/" class="placeholder__back">Vista general</RouterLink>
          para verla con todos los paneles.
        </p>
      </footer>
    </Card>
  </div>
</template>

<style scoped>
.placeholder {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-5);
  overflow-y: auto;
}

.placeholder__card {
  width: min(100%, 640px);
}

.placeholder__head {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin-bottom: var(--space-5);
}

.placeholder__status {
  align-self: flex-start;
  font-size: 10px;
  letter-spacing: 2px;
  text-transform: uppercase;
  padding: 3px var(--space-2);
  border: 1px solid var(--border-strong);
  color: var(--amber);
  border-radius: var(--radius-sm);
}

.placeholder__status--wip {
  color: var(--cyan);
  border-color: var(--cyan);
}

.placeholder__status--ready {
  color: var(--green);
  border-color: var(--green);
  text-shadow: 0 0 6px var(--green-glow);
}

.placeholder__title {
  font-family: 'Major Mono Display', monospace;
  font-size: 22px;
  letter-spacing: 5px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  font-weight: normal;
}

.placeholder__tagline {
  color: var(--text-dim);
  font-size: 13px;
  line-height: 1.6;
}

.placeholder__preview-title {
  font-family: 'Major Mono Display', monospace;
  font-size: 11px;
  letter-spacing: 2px;
  color: var(--text-dim);
  text-transform: uppercase;
  margin-bottom: var(--space-3);
  font-weight: normal;
}

.placeholder__list {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  font-size: 13px;
  color: var(--text);
  line-height: 1.5;
}

.placeholder__list li {
  padding-left: var(--space-4);
  position: relative;
}

.placeholder__list li::before {
  content: '›';
  position: absolute;
  left: 0;
  color: var(--amber);
  font-weight: bold;
}

.placeholder__foot {
  margin-top: var(--space-5);
  padding-top: var(--space-4);
  border-top: 1px dashed var(--border);
  font-size: 12px;
  color: var(--text-dim);
  line-height: 1.5;
}

.placeholder__back {
  color: var(--amber);
  border-bottom: 1px dashed var(--amber);
}
</style>
