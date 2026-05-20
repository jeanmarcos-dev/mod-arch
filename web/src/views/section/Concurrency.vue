<script setup lang="ts">
import SectionPlaceholder from './SectionPlaceholder.vue';
</script>

<template>
  <SectionPlaceholder
    title="Concurrencia"
    tagline="Sub-fase futura: cuando agreguemos un segundo core al simulador, esta sección visualizará mutex, semáforos y race conditions."
    status="soon"
    :preview="[
      'Dos columnas CPU0 / CPU1 con timeline paralela',
      'Demos pedagógicos: race_counter sin lock vs safe_counter con mutex',
      'Visualización de la ReadyQueue compartida (lock acquire/release)',
      'Producer-consumer con semáforos animado',
      'Dining philosophers con visual de deadlock',
    ]"
  />
</template>
