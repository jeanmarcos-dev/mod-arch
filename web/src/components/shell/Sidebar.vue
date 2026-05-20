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

withDefaults(
  defineProps<{
    /** Items to render. Sections precede tools; both share the same shape. */
    items: NavItem[];
    /** Heading shown when the sidebar is expanded. */
    heading: string;
  }>(),
  {},
);

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
    <header class="sidebar__head">
      <button
        type="button"
        class="sidebar__toggle"
        :title="collapsed ? 'Expandir menú' : 'Contraer menú'"
        :aria-label="collapsed ? 'Expandir menú' : 'Contraer menú'"
        @click="prefs.toggleSidebar()"
      >
        <span aria-hidden="true">{{ collapsed ? '»' : '«' }}</span>
      </button>
      <h1 v-if="!collapsed" class="sidebar__brand">{{ heading }}</h1>
    </header>

    <nav class="sidebar__nav" aria-label="Navegación principal">
      <template v-for="(item, idx) in items" :key="item.to">
        <div
          v-if="item.group && (idx === 0 || items[idx - 1].group !== item.group)"
          class="sidebar__group"
        >
          <span v-if="!collapsed">{{ item.group }}</span>
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
          <span v-if="!collapsed" class="sidebar__label">
            {{ item.label }}
            <span v-if="item.soon" class="sidebar__soon">Próx.</span>
          </span>
        </RouterLink>
      </template>
    </nav>

    <footer class="sidebar__foot">
      <div v-if="!collapsed && auth.user" class="sidebar__user">
        <span class="sidebar__user-icon" aria-hidden="true">[👤]</span>
        <span class="sidebar__user-name">{{ auth.user.name }}</span>
      </div>
    </footer>
  </aside>
</template>

<style scoped>
.sidebar {
  display: flex;
  flex-direction: column;
  background: var(--surface-1);
  border-right: 1px solid var(--border);
  width: 220px;
  flex-shrink: 0;
  transition: width var(--transition-base);
  overflow: hidden;
}

.sidebar--collapsed {
  width: 64px;
}

.sidebar__head {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-3);
  border-bottom: 1px dashed var(--border);
  min-height: 48px;
}

.sidebar__toggle {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--amber);
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  font-family: inherit;
  font-size: 13px;
  border-radius: var(--radius-sm);
  transition: background var(--transition-fast), box-shadow var(--transition-fast);
  flex-shrink: 0;
}

.sidebar__toggle:hover {
  background: rgba(255, 179, 71, 0.12);
  box-shadow: var(--shadow-glow-amber);
}

.sidebar__brand {
  font-family: 'Major Mono Display', monospace;
  font-size: 13px;
  letter-spacing: 3px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  font-weight: normal;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sidebar__nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: var(--space-2) 0;
  flex: 1;
  overflow-y: auto;
}

.sidebar__group {
  padding: var(--space-3) var(--space-3) var(--space-1);
  font-size: 10px;
  letter-spacing: 2px;
  color: var(--text-muted);
  text-transform: uppercase;
  white-space: nowrap;
  /* Fixed height across both states so toggling the rail does not move
     adjacent items up or down. */
  min-height: 30px;
  display: flex;
  align-items: flex-end;
}

/* Collapsed rail: hide the label text but keep the same row height as the
   expanded state; render a horizontal dashed divider centered vertically. */
.sidebar--collapsed .sidebar__group {
  padding: 0 var(--space-3);
  border-top: 1px dashed var(--border);
  margin: 0 var(--space-2);
  min-height: 30px;
  align-items: center;
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
    border-color var(--transition-fast);
  position: relative;
  /* Constant row height across expanded/collapsed states avoids visual jumps
     when the rail toggles. The 38px target is large enough for touch while
     keeping the rail compact. */
  min-height: 38px;
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
  width: 20px;
  text-align: center;
  font-size: 16px;
  line-height: 1;
  flex-shrink: 0;
  color: inherit;
}

/* Collapsed rail: the icon becomes the whole content. Keep the same
   min-height so toggling expand/collapse doesn't shift row positions. */
.sidebar--collapsed .sidebar__link {
  justify-content: center;
  padding: 0;
}

.sidebar--collapsed .sidebar__icon {
  width: 100%;
  font-size: 18px;
}

.sidebar__label {
  display: flex;
  align-items: baseline;
  gap: var(--space-2);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  letter-spacing: 0.5px;
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
  font-size: 11px;
  min-height: 36px;
}

.sidebar__user {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  color: var(--text-dim);
}

.sidebar__user-icon {
  color: var(--amber);
  font-family: 'Major Mono Display', monospace;
  font-size: 11px;
}

.sidebar__user-name {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

@media (max-width: 768px) {
  .sidebar {
    width: 64px;
  }
  .sidebar:not(.sidebar--collapsed) .sidebar__brand,
  .sidebar:not(.sidebar--collapsed) .sidebar__label,
  .sidebar:not(.sidebar--collapsed) .sidebar__group span,
  .sidebar:not(.sidebar--collapsed) .sidebar__user {
    display: none;
  }
}
</style>
