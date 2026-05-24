<script setup lang="ts">
import { computed } from 'vue';
import { Menu, X } from '@lucide/vue';
import { usePreferencesStore } from '../../stores/preferences';
import { useConnectionBanner } from '../../composables/useConnectionBanner';

const prefs = usePreferencesStore();
const { visible: bannerVisible } = useConnectionBanner();

const open = computed(() => prefs.sidebarMobileOpen);
const shiftedDown = computed(() => bannerVisible.value);
</script>

<template>
  <button
    type="button"
    class="mobile-menu-btn"
    :class="{ 'mobile-menu-btn--open': open, 'mobile-menu-btn--shifted': shiftedDown }"
    :aria-label="open ? 'Cerrar menú' : 'Abrir menú'"
    :aria-expanded="open"
    @click="prefs.toggleSidebarMobile()"
  >
    <component :is="open ? X : Menu" :size="18" :stroke-width="2" />
  </button>
</template>

<style scoped>
.mobile-menu-btn {
  display: none;
  position: fixed;
  top: 8px;
  left: 8px;
  width: 36px;
  height: 36px;
  align-items: center;
  justify-content: center;
  background: var(--surface-2);
  border: 1px solid var(--accent);
  color: var(--accent);
  border-radius: var(--radius-sm);
  cursor: pointer;
  padding: 0;
  z-index: calc(var(--z-overlay) + 1);
  box-shadow: var(--shadow-glow-amber);
  transition: background var(--transition-fast), color var(--transition-fast);
}

.mobile-menu-btn:hover {
  background: rgba(var(--accent-rgb), 0.18);
}

.mobile-menu-btn:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px rgba(var(--accent-rgb), 0.45);
}

.mobile-menu-btn svg { display: block; }

@media (max-width: 480px) {
  .mobile-menu-btn { display: inline-flex; }
}

/* While the connection banner is visible the shell reserves vertical space
   for it — slide the button down so it does not sit underneath the alert. */
.mobile-menu-btn--shifted {
  top: calc(28px + 4px);
}
</style>
