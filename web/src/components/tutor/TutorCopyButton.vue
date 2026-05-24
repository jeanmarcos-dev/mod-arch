<script setup lang="ts">
import { ref } from 'vue';
import { Copy, Check } from '@lucide/vue';
import { useToastStore } from '../../stores/toast';

const props = defineProps<{
  text: string;
  size?: number;
  ariaLabel?: string;
}>();

const toast = useToastStore();
const copied = ref(false);
let resetTimer: number | undefined;

async function copy() {
  try {
    await navigator.clipboard.writeText(props.text);
    copied.value = true;
    toast.ok('Copiado al portapapeles');
    if (resetTimer !== undefined) window.clearTimeout(resetTimer);
    resetTimer = window.setTimeout(() => { copied.value = false; }, 1400);
  } catch {
    toast.err('No se pudo copiar al portapapeles');
  }
}
</script>

<template>
  <button
    type="button"
    class="copy-btn"
    :class="{ copied }"
    :title="ariaLabel || 'Copiar'"
    :aria-label="ariaLabel || 'Copiar'"
    @click.stop="copy"
  >
    <Check v-if="copied" :size="size ?? 12" :stroke-width="2" />
    <Copy v-else :size="size ?? 12" :stroke-width="2" />
  </button>
</template>

<style scoped>
.copy-btn {
  background: var(--surface-overlay);
  border: 1px solid rgba(var(--accent-rgb), 0.3);
  color: var(--accent);
  border-radius: 0.25rem;
  padding: 0.15rem 0.3rem;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: background var(--transition-fast, 0.15s), border-color var(--transition-fast, 0.15s);
}
.copy-btn:hover {
  background: rgba(var(--accent-rgb), 0.18);
  border-color: rgba(var(--accent-rgb), 0.6);
}
.copy-btn.copied {
  color: var(--green);
  border-color: rgba(127, 255, 170, 0.6);
  background: rgba(127, 255, 170, 0.12);
}
</style>
