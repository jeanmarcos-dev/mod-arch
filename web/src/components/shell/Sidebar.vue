<script setup lang="ts">
import { computed, type Component } from 'vue';
import { RouterLink, useRoute } from 'vue-router';
import { ChevronLeft, ChevronRight, UserRound } from '@lucide/vue';
import { usePreferencesStore } from '../../stores/preferences';
import { useAuthStore } from '../../stores/auth';

interface NavItem {
  to: string;
  label: string;
  icon: Component;
  group?: string;
  soon?: boolean;
}

const props = defineProps<{
  /** Items to render. Sections precede tools; both share the same shape. */
  items: NavItem[];
}>();

const prefs = usePreferencesStore();
const auth = useAuthStore();
const route = useRoute();

const collapsed = computed(() => prefs.sidebarCollapsed);
const mobileOpen = computed(() => prefs.sidebarMobileOpen);

// activeTo picks the most specific item whose `to` matches the current path.
// Without this, items whose `to` is a prefix of another (e.g. `/admin` vs
// `/admin/usuarios`) would both light up — the root item would always look
// active even when standing on a child route.
const activeTo = computed<string | null>(() => {
  const path = route.path;
  if (path === '/') return props.items.some((i) => i.to === '/') ? '/' : null;
  let best: NavItem | null = null;
  for (const item of props.items) {
    if (item.to === '/') continue;
    const matches = path === item.to || path.startsWith(item.to + '/');
    if (!matches) continue;
    if (!best || item.to.length > best.to.length) best = item;
  }
  return best?.to ?? null;
});

function isActive(to: string): boolean {
  return activeTo.value === to;
}

function onLinkClick(item: NavItem, e: MouseEvent) {
  if (item.soon) {
    e.preventDefault();
    return;
  }
  if (prefs.sidebarMobileOpen) prefs.closeSidebarMobile();
}
</script>

<template>
  <div
    v-if="mobileOpen"
    class="sidebar__backdrop"
    aria-hidden="true"
    @click="prefs.closeSidebarMobile()"
  />
  <aside
    class="sidebar"
    :class="{ 'sidebar--collapsed': collapsed, 'sidebar--mobile-open': mobileOpen }"
    data-pizarra-anchor="sidebar"
  >
    <nav class="sidebar__nav" aria-label="Navegación principal">
      <template v-for="(item, idx) in items" :key="item.to">
        <div
          v-if="item.group && (idx === 0 || items[idx - 1].group !== item.group)"
          class="sidebar__group"
        >
          <span class="sidebar__group-label">{{ item.group }}</span>
        </div>

        <RouterLink
          :to="item.to"
          class="sidebar__link"
          :class="{
            'sidebar__link--active': isActive(item.to),
            'sidebar__link--soon': item.soon,
          }"
          :title="collapsed ? item.label : undefined"
          :aria-disabled="item.soon ? 'true' : undefined"
          @click="(e) => onLinkClick(item, e)"
        >
          <span class="sidebar__icon" aria-hidden="true">
            <component :is="item.icon" :size="collapsed ? 20 : 16" :stroke-width="2" />
          </span>
          <span class="sidebar__label">
            {{ item.label }}
            <span v-if="item.soon" class="sidebar__soon">Próx.</span>
          </span>
        </RouterLink>
      </template>
    </nav>

    <footer class="sidebar__foot">
      <div v-if="auth.user" class="sidebar__user">
        <UserRound :size="14" :stroke-width="2" class="sidebar__user-icon" />
        <span class="sidebar__user-name">{{ auth.user.name }}</span>
      </div>
      <span class="sidebar__brand" :aria-hidden="collapsed ? 'true' : undefined">
        <span class="sidebar__brand-full">MODOS-CPU</span>
        <span class="sidebar__brand-mark">M·C</span>
      </span>
    </footer>

    <button
      type="button"
      class="sidebar__toggle"
      :title="collapsed ? 'Expandir menú' : 'Contraer menú'"
      :aria-label="collapsed ? 'Expandir menú' : 'Contraer menú'"
      @click="prefs.toggleSidebar()"
    >
      <component :is="collapsed ? ChevronRight : ChevronLeft" :size="16" :stroke-width="2.5" />
    </button>
  </aside>
</template>

<style scoped>
.sidebar {
  position: relative;
  display: flex;
  flex-direction: column;
  background: var(--surface-1);
  border-right: 1px solid var(--border);
  width: 220px;
  flex-shrink: 0;
  transition: width var(--transition-base);
  /* Allow the bottom toggle handle to overhang onto the seam between this
     panel and the main content. Inner children that need clipping (nav, foot)
     manage their own overflow. */
}

.sidebar--collapsed {
  width: 56px;
}

/* Toggle handle straddles the right border so it sits on the seam between
   sidebar and main content. Bottom-anchored as the user requested. */
.sidebar__toggle {
  position: absolute;
  right: -13px;
  bottom: var(--space-4);
  width: 26px;
  height: 26px;
  background: var(--surface-2);
  border: 1px solid var(--amber);
  color: var(--amber);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  font-family: inherit;
  font-size: 13px;
  border-radius: var(--radius-sm);
  z-index: 10;
  padding: 0;
  box-shadow: 0 0 0 1px var(--surface-0), var(--shadow-glow-amber);
  transition: background var(--transition-fast), box-shadow var(--transition-fast), border-color var(--transition-fast), transform var(--transition-fast);
}

.sidebar__toggle:hover {
  background: rgba(var(--accent-rgb), 0.22);
  transform: scale(1.08);
}

.sidebar__toggle:focus-visible {
  outline: none;
  box-shadow: 0 0 0 1px var(--surface-0), 0 0 0 3px rgba(var(--accent-rgb), 0.45);
}

.sidebar__toggle svg { display: block; stroke-width: 2.5; }

.sidebar__nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 0 0 var(--space-2);
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
}

.sidebar__group {
  padding: var(--space-3) var(--space-3) var(--space-1);
  white-space: nowrap;
  /* Fixed height across both states so toggling the rail does not move
     adjacent items up or down. */
  min-height: 30px;
  display: flex;
  align-items: flex-end;
  position: relative;
  transition: padding var(--transition-base);
}

/* The dashed divider appears only in the collapsed state. It lives as a
   pseudo-element so its opacity can fade in alongside the width change
   instead of popping in at the end of the transition. */
.sidebar__group::after {
  content: '';
  position: absolute;
  left: var(--space-2);
  right: var(--space-2);
  top: 50%;
  border-top: 1px dashed var(--border);
  opacity: 0;
  transition: opacity var(--transition-base);
  pointer-events: none;
}

.sidebar__group-label {
  font-size: 10px;
  letter-spacing: 2px;
  color: var(--text-muted);
  text-transform: uppercase;
  overflow: hidden;
  max-width: 200px;
  opacity: 1;
  transition:
    max-width var(--transition-base),
    opacity var(--transition-fast);
}

.sidebar--collapsed .sidebar__group {
  padding: 0 var(--space-2);
  align-items: center;
}

.sidebar--collapsed .sidebar__group-label {
  max-width: 0;
  opacity: 0;
}

.sidebar--collapsed .sidebar__group::after {
  opacity: 1;
}

.sidebar__link {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: 0 var(--space-3);
  color: var(--text-dim);
  text-decoration: none;
  font-size: 13px;
  border-left: 2px solid transparent;
  transition:
    background var(--transition-fast),
    color var(--transition-fast),
    border-color var(--transition-fast),
    padding var(--transition-base),
    gap var(--transition-base);
  position: relative;
  min-height: 38px;
  overflow: hidden;
}

.sidebar__link:hover {
  background: rgba(var(--accent-rgb), 0.06);
  color: var(--amber);
  text-shadow: none;
}

.sidebar__link--active {
  background: rgba(var(--accent-rgb), 0.10);
  color: var(--amber);
  border-left-color: var(--amber);
  box-shadow: inset 4px 0 12px -8px rgba(var(--accent-rgb), 0.6);
}

.sidebar__link--soon {
  opacity: 0.45;
  cursor: not-allowed;
}

.sidebar__link--soon:hover {
  background: transparent;
  color: var(--text-dim);
}

.sidebar__icon {
  width: 22px;
  height: 20px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  color: inherit;
  transition: width var(--transition-base);
}
.sidebar__icon svg { display: block; }

.sidebar__label {
  display: flex;
  align-items: baseline;
  gap: var(--space-2);
  white-space: nowrap;
  overflow: hidden;
  letter-spacing: 0.5px;
  flex: 1;
  min-width: 0;
  max-width: 200px;
  opacity: 1;
  transition:
    max-width var(--transition-base),
    opacity var(--transition-fast);
}

/* Collapsed rail: the icon becomes the whole content. The same min-height
   keeps row Y positions identical so siblings don't jump during the
   width transition. */
.sidebar--collapsed .sidebar__link {
  justify-content: center;
  padding: 0;
  gap: 0;
}

.sidebar--collapsed .sidebar__icon {
  width: 100%;
  font-size: 20px;
}

.sidebar--collapsed .sidebar__label {
  max-width: 0;
  opacity: 0;
}

.sidebar__soon {
  font-size: 9px;
  letter-spacing: 1px;
  color: var(--text-muted);
  text-transform: uppercase;
  border: 1px solid var(--border);
  padding: 1px var(--space-1);
  border-radius: var(--radius-sm);
}

.sidebar__foot {
  border-top: 1px dashed var(--border);
  padding: var(--space-3);
  /* Leave room above the absolute-positioned toggle handle so the user chip
     never overlaps it. */
  padding-bottom: calc(var(--space-3) + 26px + var(--space-3));
  font-size: 11px;
  min-height: 36px;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.sidebar__user {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  color: var(--text-dim);
  overflow: hidden;
}

.sidebar__user-icon {
  color: var(--amber);
  flex-shrink: 0;
}

.sidebar__user-name {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 160px;
  opacity: 1;
  transition:
    max-width var(--transition-base),
    opacity var(--transition-fast);
}

.sidebar--collapsed .sidebar__user-name {
  max-width: 0;
  opacity: 0;
}

.sidebar--collapsed .sidebar__foot {
  padding: var(--space-2) 0;
  padding-bottom: calc(var(--space-2) + 26px + var(--space-3));
  align-items: center;
}

/* Subtle signature line at the very bottom of the sidebar. Full wordmark
   when expanded; collapses to a short "M·C" monogram when the sidebar rail
   is in collapsed mode so it still anchors the brand without overflowing
   the narrow column. */
.sidebar__brand {
  font-family: var(--font-display);
  font-size: 9px;
  letter-spacing: 4px;
  text-transform: uppercase;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  opacity: 0.55;
  white-space: nowrap;
  user-select: none;
  text-align: center;
  transition: opacity var(--transition-fast);
}
.sidebar__brand:hover { opacity: 0.85; }
.sidebar__brand-mark { display: none; }
.sidebar--collapsed .sidebar__brand-full { display: none; }
.sidebar--collapsed .sidebar__brand-mark {
  display: inline;
  letter-spacing: 2px;
}

@media (max-width: 768px) {
  /* On mobile the rail always behaves like the collapsed state — the
     transition animation still runs identically when toggling. */
  .sidebar { width: 56px; }
}

/* Phone portrait: the icon-rail becomes an off-canvas drawer that the
   hamburger in the topbar opens. The expanded width returns so labels are
   readable while the drawer is open. The drawer outranks the tutor FAB
   (--z-modal = 600) so a primary nav action is never trapped underneath a
   secondary overlay — sits at 650, below toast (700). */
@media (max-width: 480px) {
  .sidebar {
    position: fixed;
    inset: 0 auto 0 0;
    width: 220px;
    transform: translateX(-100%);
    transition: transform var(--transition-base);
    z-index: 650;
    box-shadow: 8px 0 24px rgba(0, 0, 0, 0.45);
  }

  .sidebar--mobile-open {
    transform: translateX(0);
  }

  /* The desktop collapse handle has no meaning in drawer mode. */
  .sidebar__toggle { display: none; }
}

.sidebar__backdrop {
  position: fixed;
  inset: 0;
  background: var(--surface-overlay-strong);
  z-index: 649;
  animation: sidebar-backdrop-in var(--transition-fast) ease-out;
}

@keyframes sidebar-backdrop-in {
  from { opacity: 0; }
  to   { opacity: 1; }
}
</style>
