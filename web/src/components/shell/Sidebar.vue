<script setup lang="ts">
import { computed } from 'vue';
import { RouterLink, useRoute } from 'vue-router';
import { usePreferencesStore } from '../../stores/preferences';
import { useAuthStore } from '../../stores/auth';

interface NavItem {
  to: string;
  label: string;
  icon: string;
  group?: string;
  soon?: boolean;
}

defineProps<{
  /** Items to render. Sections precede tools; both share the same shape. */
  items: NavItem[];
}>();

const prefs = usePreferencesStore();
const auth = useAuthStore();
const route = useRoute();

const collapsed = computed(() => prefs.sidebarCollapsed);

function isActive(to: string): boolean {
  if (to === '/') return route.path === '/';
  return route.path === to || route.path.startsWith(to + '/');
}
</script>

<template>
  <aside class="sidebar" :class="{ 'sidebar--collapsed': collapsed }">
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
          @click="(e) => item.soon && e.preventDefault()"
        >
          <span class="sidebar__icon" aria-hidden="true">{{ item.icon }}</span>
          <span class="sidebar__label">
            {{ item.label }}
            <span v-if="item.soon" class="sidebar__soon">Próx.</span>
          </span>
        </RouterLink>
      </template>
    </nav>

    <footer class="sidebar__foot">
      <div v-if="auth.user" class="sidebar__user">
        <span class="sidebar__user-icon" aria-hidden="true">[👤]</span>
        <span class="sidebar__user-name">{{ auth.user.name }}</span>
      </div>
    </footer>

    <button
      type="button"
      class="sidebar__toggle"
      :title="collapsed ? 'Expandir menú' : 'Contraer menú'"
      :aria-label="collapsed ? 'Expandir menú' : 'Contraer menú'"
      @click="prefs.toggleSidebar()"
    >
      <span aria-hidden="true">{{ collapsed ? '»' : '«' }}</span>
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
  border: 1px solid var(--border-strong);
  color: var(--amber);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  font-family: inherit;
  font-size: 13px;
  border-radius: var(--radius-sm);
  z-index: 10;
  transition: background var(--transition-fast), box-shadow var(--transition-fast), border-color var(--transition-fast);
}

.sidebar__toggle:hover {
  background: rgba(255, 179, 71, 0.18);
  border-color: var(--amber);
  box-shadow: var(--shadow-glow-amber);
}

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
  background: rgba(255, 179, 71, 0.06);
  color: var(--amber);
  text-shadow: none;
}

.sidebar__link--active {
  background: rgba(255, 179, 71, 0.10);
  color: var(--amber);
  border-left-color: var(--amber);
  box-shadow: inset 4px 0 12px -8px rgba(255, 179, 71, 0.6);
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
  font-family: 'JetBrains Mono', 'Major Mono Display', monospace;
  width: 22px;
  text-align: center;
  font-size: 16px;
  line-height: 1;
  flex-shrink: 0;
  color: inherit;
  transition:
    font-size var(--transition-base),
    width var(--transition-base);
}

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
  font-family: 'Major Mono Display', monospace;
  font-size: 11px;
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
  display: flex;
  justify-content: center;
}

@media (max-width: 768px) {
  /* On mobile the rail always behaves like the collapsed state — the
     transition animation still runs identically when toggling. */
  .sidebar { width: 56px; }
}
</style>
