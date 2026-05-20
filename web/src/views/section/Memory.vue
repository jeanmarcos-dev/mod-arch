<script setup lang="ts">
import SectionPlaceholder from './SectionPlaceholder.vue';
</script>

<template>
  <SectionPlaceholder
    title="Memoria"
    tagline="Vista con foco en la memoria del sistema: cómo viajan los bytes entre CPU y RAM, qué regiones se tocan más, dónde vive cada proceso."
    status="soon"
    :preview="[
      'Hex viewer extra grande con auto-scroll al punto activo',
      'Animación de bytes viajando del CPU a memoria en cada write',
      'Mapa de calor de regiones más accedidas',
      'Layout del proceso: text/data/stack/heap con tamaños reales',
      'Diff visual cuando una zona cambia entre snapshots',
    ]"
  />
</template>
