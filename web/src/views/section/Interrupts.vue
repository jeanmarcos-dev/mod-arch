<script setup lang="ts">
import SectionPlaceholder from './SectionPlaceholder.vue';
</script>

<template>
  <SectionPlaceholder
    title="Interrupciones"
    tagline="Vista con foco en interrupciones y cambios de modo: cuándo entra el kernel, qué hace la IDT, cómo vuelve el control al proceso."
    status="soon"
    :preview="[
      'IDT desplegada con entradas resaltadas al dispararse',
      'Fade visual USER → KERNEL del fondo en cada int_enter',
      'Stack de retorno y EIP/CS/EFLAGS guardados',
      'Línea de tiempo de últimas interrupciones (timer, keyboard, syscall)',
      'Animación del IRET volviendo a USER',
    ]"
  />
</template>
