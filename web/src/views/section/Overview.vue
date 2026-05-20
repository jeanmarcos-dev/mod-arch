<script setup lang="ts">
// Overview wraps the existing Simulator view so it can be mounted as a child
// route of AppShell without touching the dense panel layout.
import Simulator from '../Simulator.vue';
</script>

<template>
  <Simulator />
</template>
