<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { Menu, X } from '@lucide/vue';
import { usePreferencesStore } from '../../stores/preferences';
import { useConnectionBanner } from '../../composables/useConnectionBanner';

const { t } = useI18n();
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
    :aria-label="open ? t('shell.mobileMenu.close') : t('shell.mobileMenu.open')"
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
  /* Geometry matches .sim-bar__right .icon-btn (28×28 on mobile) so the
     hamburger is the same size as the tools / expand toggles it sits
     across the SimBar from. `top` mirrors the SimBar row's
     `padding-top: var(--space-2)` so the icon shares the vertical
     baseline with TICK/WS and the settings/cores cluster. */
  top: 8px;
  left: 8px;
  width: 28px;
  height: 28px;
  /* Neutralise the global mobile-touch override (`button { min-height: 44px }`
     in style.css) that would otherwise stretch the hamburger to 44px and push
     it into the row below. */
  min-height: 0;
  align-items: center;
  justify-content: center;
  background: var(--surface-2);
  border: 1px solid var(--border-strong);
  color: var(--accent);
  border-radius: var(--radius-sm);
  cursor: pointer;
  padding: 0;
  /* Sits one layer above the drawer (650) so the X to close it is always
     reachable even while the panel itself covers the rest of the UI. */
  z-index: 651;
  /* No drop-shadow when docked — the muted border is enough next to the
     SimBar buttons and avoids the "floating card" look that read as a
     disconnected column. The glow returns only while the drawer is open so
     the close affordance still pops. */
  transition: background var(--transition-fast), color var(--transition-fast),
    border-color var(--transition-fast), box-shadow var(--transition-fast),
    left var(--transition-base);
}

.mobile-menu-btn:hover {
  background: rgba(var(--accent-rgb), 0.18);
}

/* Drawer open: slide the toggle to the right edge of the drawer so the X
   sits over the dim backdrop instead of overlapping the first group label
   ("SIMULACIÓN") inside the menu. The drawer itself uses
   `clamp(220px, 75vw, 280px)`, so we follow it with the same formula plus
   an 8px gutter so the button hugs the drawer's right edge across all
   mobile viewports (320 → 768). */
.mobile-menu-btn--open {
  left: clamp(228px, calc(75vw + 8px), 288px);
  border-color: var(--accent);
  box-shadow: var(--shadow-glow-amber);
}

.mobile-menu-btn:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px rgba(var(--accent-rgb), 0.45);
}

.mobile-menu-btn svg { display: block; }

@media (max-width: 768px) {
  .mobile-menu-btn { display: inline-flex; }
}

/* While the connection banner is visible the shell reserves vertical space
   for it — slide the button down so it does not sit underneath the alert. */
.mobile-menu-btn--shifted {
  top: calc(28px + 4px);
}
</style>
