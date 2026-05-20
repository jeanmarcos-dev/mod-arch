<script setup lang="ts">
import SectionPlaceholder from './SectionPlaceholder.vue';
</script>

<template>
  <SectionPlaceholder
    title="Procesos"
    tagline="Vista con foco en el ciclo de vida de los procesos: cómo nacen, cómo el scheduler los pone en CPU, cómo se copian sus PCBs durante un cambio de contexto."
    status="soon"
    :preview="[
      'Tabla de PCBs en grande con resaltado del proceso activo',
      'Grafo del scheduler Round-Robin animado entre quantum y quantum',
      'Animación de copia de PCB durante ctx_switch (registros viajando)',
      'Métricas: turnaround, waiting time, throughput',
      'Timeline horizontal de los últimos quantums',
    ]"
  />
</template>
