<script setup lang="ts">
import SectionPlaceholder from '../section/SectionPlaceholder.vue';
</script>

<template>
  <SectionPlaceholder
    title="Glosario"
    tagline="Términos del simulador y de sistemas operativos, accesibles fuera del simulador."
    status="wip"
    :preview="[
      'Reusar el contenido del GlossaryModal actual como página completa',
      'Búsqueda inline con resaltado de coincidencias',
      'Categorías: CPU, Memoria, Procesos, Interrupciones, Concurrencia',
      'Links cruzados entre términos relacionados',
      'Versión imprimible para repartir en clase',
    ]"
  />
</template>
