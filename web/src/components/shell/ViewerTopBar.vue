<script setup lang="ts">
import { computed, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { BookOpen, Hand, Eye, EyeOff, LogOut } from '@lucide/vue';
import { useCpuStore } from '../../stores/cpu';
import { useViewerStore } from '../../stores/viewer';

const { t } = useI18n();
const cpu = useCpuStore();
const viewer = useViewerStore();
const router = useRouter();

// The host alias surfaces in the "Watching:" indicator. Falls back to the
// session id when the host has not set a label yet.
const hostLabel = computed(() => {
  const lab = cpu.sessionInfo.label?.trim();
  if (lab) return lab;
  const id = cpu.sessionInfo.id;
  return id ? t('shell.viewerTopBar.fallbackSession', { id: id.slice(0, 6) }) : t('shell.viewerTopBar.fallbackTheSession');
});

const handRaisePulse = ref(false);
function onHandRaise() {
  cpu.raiseHand();
  // Visual feedback for the student: pulse the icon for ~1s. The host's toast
  // is what actually confirms delivery — this is just so the click feels alive.
  handRaisePulse.value = true;
  window.setTimeout(() => { handRaisePulse.value = false; }, 900);
}

function onToggleFollow() {
  viewer.toggleAutofollow();
}

function onGlossary() {
  cpu.openGlossary();
}

async function onLeave() {
  try {
    await fetch('/v/leave', { method: 'POST', credentials: 'same-origin' });
  } catch {
    // Network failure does not stop the user from leaving — push them out
    // anyway and let the next page render reflect the (un)cleared cookie.
  }
  // Send the student to the landing as a fresh client. A full reload is the
  // safest bet — the cookie change must take effect for the next WS connect.
  window.location.assign('/');
  // Belt-and-suspenders router push in case the assign is intercepted in
  // test/dev contexts.
  await router.push('/').catch(() => undefined);
}
</script>

<template>
  <header class="viewer-bar" role="banner" data-pizarra-anchor="viewer-topbar">
    <span class="viewer-bar__brand">{{ t('topbar.brand') }}</span>

    <button
      type="button"
      class="viewer-bar__btn"
      :class="{ 'viewer-bar__btn--on': viewer.autofollow }"
      :title="viewer.autofollow
        ? t('shell.viewerTopBar.autofollowOnHint')
        : t('shell.viewerTopBar.autofollowOffHint')"
      :aria-pressed="viewer.autofollow"
      @click="onToggleFollow"
    >
      <component :is="viewer.autofollow ? Eye : EyeOff" :size="14" :stroke-width="2" />
      <span>{{ viewer.autofollow ? t('shell.viewerTopBar.autofollowLabel') : t('shell.viewerTopBar.freeLabel') }}</span>
    </button>

    <button
      type="button"
      class="viewer-bar__btn"
      :title="t('shell.viewerTopBar.glossaryHint')"
      @click="onGlossary"
    >
      <BookOpen :size="14" :stroke-width="2" />
      <span>{{ t('shell.viewerTopBar.glossaryLabel') }}</span>
    </button>

    <button
      type="button"
      class="viewer-bar__btn viewer-bar__btn--hand"
      :class="{ 'viewer-bar__btn--pulse': handRaisePulse }"
      :title="t('shell.viewerTopBar.handRaiseHint')"
      @click="onHandRaise"
    >
      <Hand :size="14" :stroke-width="2" />
      <span>{{ t('shell.viewerTopBar.handRaiseLabel') }}</span>
    </button>

    <span class="viewer-bar__spacer" />

    <span class="viewer-bar__host" :title="t('shell.viewerTopBar.hostHint', { host: hostLabel })">
      <Eye :size="13" :stroke-width="2" class="viewer-bar__host-icon" />
      <span class="viewer-bar__host-label">{{ t('shell.viewerTopBar.watching') }} <strong>{{ hostLabel }}</strong></span>
    </span>

    <button
      type="button"
      class="viewer-bar__btn viewer-bar__btn--leave"
      :title="t('shell.viewerTopBar.leaveHint')"
      @click="onLeave"
    >
      <LogOut :size="14" :stroke-width="2" />
      <span>{{ t('shell.viewerTopBar.leaveLabel') }}</span>
    </button>
  </header>
</template>

<style scoped>
.viewer-bar {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: 0 var(--space-3);
  height: 48px;
  background: var(--surface-1);
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}

.viewer-bar__brand {
  font-family: var(--font-display);
  letter-spacing: 3px;
  color: var(--amber);
  text-shadow: var(--shadow-glow-amber);
  font-size: 12px;
  white-space: nowrap;
}

.viewer-bar__spacer { flex: 1; }

.viewer-bar__btn {
  font-family: inherit;
  background: rgba(var(--accent-rgb), 0.05);
  border: 1px solid var(--border);
  color: var(--text-dim);
  font-size: 10px;
  letter-spacing: 1.5px;
  text-transform: uppercase;
  height: 32px;
  padding: 0 12px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  box-sizing: border-box;
  cursor: pointer;
  border-radius: var(--radius-sm);
  transition: background var(--transition-fast), color var(--transition-fast), border-color var(--transition-fast), box-shadow var(--transition-fast);
  white-space: nowrap;
}
.viewer-bar__btn:hover {
  background: rgba(var(--accent-rgb), 0.12);
  color: var(--amber);
  border-color: var(--border-strong);
}
.viewer-bar__btn > svg { flex-shrink: 0; }

.viewer-bar__btn--on {
  background: rgba(127, 255, 170, 0.10);
  color: var(--green);
  border-color: var(--green);
  box-shadow: var(--shadow-glow-green);
}
.viewer-bar__btn--on:hover {
  background: rgba(127, 255, 170, 0.18);
  color: var(--green);
}

.viewer-bar__btn--hand {
  /* Mano levantada con un acento ámbar suave, distinto del autofollow para
     que cada acción tenga su propia identidad visual. */
  color: var(--amber);
  border-color: rgba(var(--accent-rgb), 0.4);
}
.viewer-bar__btn--hand:hover {
  background: rgba(var(--accent-rgb), 0.18);
  border-color: var(--amber);
}
.viewer-bar__btn--pulse {
  animation: viewer-bar-pulse 0.9s ease-out;
}

.viewer-bar__btn--leave {
  color: var(--red-soft, #ff7a7a);
  border-color: rgba(255, 122, 122, 0.35);
}
.viewer-bar__btn--leave:hover {
  background: rgba(255, 58, 58, 0.12);
  border-color: var(--red);
  color: var(--red);
}

.viewer-bar__host {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  letter-spacing: 0.5px;
  color: var(--text-dim);
  background: rgba(255, 213, 79, 0.08);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 0 10px;
  height: 28px;
  min-width: 0;
  max-width: 220px;
  overflow: hidden;
}
.viewer-bar__host-icon { color: var(--yellow); flex-shrink: 0; }
.viewer-bar__host-label {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}
.viewer-bar__host-label strong {
  color: var(--amber);
  font-weight: 600;
  letter-spacing: 1px;
  margin-left: 4px;
}

@keyframes viewer-bar-pulse {
  0%   { box-shadow: 0 0 0 0 rgba(var(--accent-rgb), 0.5); }
  100% { box-shadow: 0 0 0 12px rgba(var(--accent-rgb), 0); }
}

@media (max-width: 768px) {
  .viewer-bar {
    flex-wrap: wrap;
    height: auto;
    min-height: 44px;
    padding: var(--space-2) var(--space-3);
    gap: var(--space-2);
  }
  .viewer-bar__spacer { display: none; }
  .viewer-bar__btn > span:not(.viewer-bar__host-label) { display: none; }
  .viewer-bar__host { margin-left: auto; }
  .viewer-bar__host-label { display: none; }
}
@media (max-width: 700px) {
  .viewer-bar__brand { display: none; }
}
@media (max-width: 480px) {
  /* Make room for the floating hamburger at the top-left. */
  .viewer-bar { padding-left: 52px; }
  .viewer-bar__host { max-width: none; flex: 1 1 auto; }
}
</style>
