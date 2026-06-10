<script setup lang="ts">
import { ref, watch, computed, onMounted, onBeforeUnmount } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useCpuStore } from '../../stores/cpu'
import LoaderModarch from './LoaderModarch.vue'

const MIN_DISPLAY_MS = 1200
const MAX_DISPLAY_MS = 4500

const cpu = useCpuStore()
const route = useRoute()
const { t } = useI18n()

const isAuthRoute = computed(() => {
  const name = route.name
  return name === 'login' || name === 'admin-login'
})

const visible = ref(!isAuthRoute.value)
const minElapsed = ref(false)

let minTimer: number | null = null
let maxTimer: number | null = null

onMounted(() => {
  if (isAuthRoute.value) return
  minTimer = window.setTimeout(() => { minElapsed.value = true }, MIN_DISPLAY_MS)
  maxTimer = window.setTimeout(() => { visible.value = false }, MAX_DISPLAY_MS)
})

onBeforeUnmount(() => {
  if (minTimer !== null) window.clearTimeout(minTimer)
  if (maxTimer !== null) window.clearTimeout(maxTimer)
})

watch(
  [() => cpu.hasEverConnected, minElapsed, isAuthRoute],
  ([connected, elapsed, authRoute]) => {
    if (authRoute) {
      visible.value = false
      return
    }
    if (connected && elapsed) visible.value = false
  },
)

const label = computed(() => t('shell.bootSplash.label'))
</script>

<template>
  <transition name="boot-splash">
    <div
      v-if="visible"
      class="boot-splash"
      role="status"
      aria-live="polite"
    >
      <LoaderModarch class="boot-splash__loader" size="220px" :label="label" />
    </div>
  </transition>
</template>

<style scoped>
.boot-splash {
  position: fixed;
  inset: 0;
  z-index: 99999;
  display: flex;
  align-items: center;
  justify-content: center;
  background:
    radial-gradient(ellipse 60% 50% at 50% 50%, rgba(var(--accent-rgb), 0.06) 0%, transparent 70%),
    var(--bg-0, #07090e);
}

.boot-splash__loader {
  --lm-label-color: var(--text-dim);
}

.boot-splash-enter-active,
.boot-splash-leave-active {
  transition: opacity 320ms var(--ease-out-soft, ease-out);
}

.boot-splash-enter-from,
.boot-splash-leave-to {
  opacity: 0;
}
</style>
