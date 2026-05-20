<script setup lang="ts">
import SectionPlaceholder from '../section/SectionPlaceholder.vue';
</script>

<template>
  <SectionPlaceholder
    title="Mis sesiones"
    tagline="Lista de sus sesiones activas y pasadas, accesos compartidos y enlaces de invitación generados."
    status="soon"
    :preview="[
      'Tabla con sus sesiones (ID, alias, modo, última actividad)',
      'Reabrir una sesión existente con un click',
      'Enlace público (mc_view) si la sesión está compartida',
      'Historial de invitaciones consumidas con su correo',
      'Botón para cerrar sesiones inactivas',
    ]"
  />
</template>
