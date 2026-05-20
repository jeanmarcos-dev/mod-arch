<script setup lang="ts">
import SectionPlaceholder from './SectionPlaceholder.vue';
</script>

<template>
  <SectionPlaceholder
    title="x86 real"
    tagline="Sub-fase futura: ejecutar binarios reales compilados con gcc-i686-linux-gnu sobre nuestro loader ELF, mostrando la traducción al ISA simplificado."
    status="soon"
    :preview="[
      'Editor de código C inline con compilación contra gcc',
      'ELF parser visual: headers, secciones, símbolos',
      'Vista lado a lado: C original ↔ x86 generado ↔ trace del simulador',
      'Demos: hello world, fibonacci, syscall write',
      'Stub minimal de libc con syscalls Linux compatibles',
    ]"
  />
</template>
