import { defineStore } from 'pinia';
import { computed, ref, watch } from 'vue';
import { setI18nLocale, applyLocale, readStoredLocale as i18nReadStoredLocale } from '../i18n';
import { DEFAULT_LOCALE, SUPPORTED_LOCALES, type Locale } from '../i18n/schema';

export type { Locale } from '../i18n/schema';

// PresentationMode tunes density, animation speed and narrator visibility in
// one place. The three presets are exposed to the user as "preferences" so a
// teacher can flip a switch before class without remembering which knobs to
// turn.
export type PresentationMode = 'compact' | 'pedagogic' | 'projector';

export type Density = 'comfortable' | 'compact';

// Theme controls the color palette + font family of the whole shell. The
// active theme is reflected on <html data-theme="..."> so it reaches portals
// and modals mounted outside the shell root.
export type Theme = 'amber' | 'green' | 'dark' | 'accesible';

// PanelName enumerates the side panels whose collapse state is persisted
// across reloads. Center column panels are tab-driven, not collapsible.
export type PanelName = 'cores' | 'processes' | 'queues' | 'ipc';

export interface PanelsCollapsed {
  cores: boolean;
  processes: boolean;
  queues: boolean;
  ipc: boolean;
}

export interface ColumnWidths {
  leftPx: number;
  rightPx: number;
}

export type ColumnSide = 'left' | 'right';

// PanelHeights stores a flex-grow weight per resizable row. The right column
// stack (Procesos/Colas/IPC) uses three weights, and the center column splits
// vertical space between the tab pile (Código/Bitácora/Memoria) and the
// console below it via two extra weights. Drag of the horizontal handles
// rewrites the weights so the user can rebalance any pair. Cores lives alone
// in the left column, so no weight is tracked for it.
export type StackPanelName = 'processes' | 'queues' | 'ipc' | 'middleTabs' | 'middleConsole';

export interface PanelHeights {
  processes: number;
  queues: number;
  ipc: number;
  middleTabs: number;
  middleConsole: number;
}

export interface LayoutSnapshot {
  panelsCollapsed: PanelsCollapsed;
  columnWidths: ColumnWidths;
  panelHeights: PanelHeights;
}

export const PANEL_HEIGHT_MIN_WEIGHT = 0.15;

interface PrefsState {
  sidebarCollapsed: boolean;
  // showInit controls whether the init process (PID 1, always present in the
  // background) appears in the PROCESOS tree, the queues panel and narrator
  // animations. The default is recomputed on each demo load via
  // applyShowInitDefault — but ONLY while the user has not touched the toggle:
  // once setShowInit fires, showInitTouched flips and subsequent demos respect
  // the user's pick instead of resetting it.
  showInit: boolean;
  showInitTouched: boolean;
  mode: PresentationMode;
  density: Density;
  narratorsEnabled: boolean;
  // Below this tick interval (ms) narrators stop firing. Pedagogically:
  // visual narration of every event is meaningless once the CPU ticks faster
  // than the eye can resolve. Configurable so the teacher can widen or
  // narrow the readable band.
  narratorMinTickMs: number;
  theme: Theme;
  locale: Locale;
  panelsCollapsed: PanelsCollapsed;
  columnWidths: ColumnWidths;
  panelHeights: PanelHeights;
}

const STORAGE_KEY = 'modos-cpu:prefs:v1';

// Minimum widths in pixels for each side column. The left column only hosts
// the Cores panel, which needs less horizontal room than the right stack of
// Procesos/Colas/IPC.
export const COLUMN_MIN_PX: Record<ColumnSide, number> = {
  left: 140,
  right: 240,
};

// Cap each side column at one third of the viewport so the center never gets
// squeezed below ~34%.
export const COLUMN_MAX_PERCENT = 0.33;

// Pedagogic is the default preset so a fresh visitor sees the animated
// transitions out of the box (animationsEnabled is derived from mode in
// the computed below). Narrators default on too: the simulator is built
// around explaining what the CPU is doing, and the animated overlays are
// the most direct way to surface that — a first-time visitor should see
// them straight away rather than having to discover the toggle.
const DEFAULTS: PrefsState = {
  sidebarCollapsed: false,
  showInit: true,
  showInitTouched: false,
  mode: 'pedagogic',
  density: 'comfortable',
  narratorsEnabled: true,
  narratorMinTickMs: 200,
  theme: 'amber',
  locale: DEFAULT_LOCALE,
  panelsCollapsed: {
    cores: false,
    processes: false,
    queues: false,
    ipc: false,
  },
  columnWidths: {
    leftPx: 280,
    rightPx: 400,
  },
  panelHeights: {
    processes: 1,
    queues: 1,
    ipc: 1,
    // Console arranca un poco más alta que la pila de tabs para preservar la
    // sensación previa al resizer (cuando ConsolePanel crecía con el contenido).
    middleTabs: 1,
    middleConsole: 1.2,
  },
};

function sanitizePanelsCollapsed(input: unknown): PanelsCollapsed {
  const base: PanelsCollapsed = { ...DEFAULTS.panelsCollapsed };
  if (!input || typeof input !== 'object') return base;
  const src = input as Record<string, unknown>;
  for (const k of Object.keys(base) as PanelName[]) {
    if (typeof src[k] === 'boolean') base[k] = src[k] as boolean;
  }
  return base;
}

function sanitizeColumnWidths(input: unknown): ColumnWidths {
  const base: ColumnWidths = { ...DEFAULTS.columnWidths };
  if (!input || typeof input !== 'object') return base;
  const src = input as Record<string, unknown>;
  if (typeof src.leftPx === 'number' && Number.isFinite(src.leftPx)) {
    base.leftPx = Math.max(COLUMN_MIN_PX.left, src.leftPx);
  }
  if (typeof src.rightPx === 'number' && Number.isFinite(src.rightPx)) {
    base.rightPx = Math.max(COLUMN_MIN_PX.right, src.rightPx);
  }
  return base;
}

function sanitizePanelHeights(input: unknown): PanelHeights {
  const base: PanelHeights = { ...DEFAULTS.panelHeights };
  if (!input || typeof input !== 'object') return base;
  const src = input as Record<string, unknown>;
  for (const k of Object.keys(base) as StackPanelName[]) {
    const v = src[k];
    if (typeof v === 'number' && Number.isFinite(v) && v > 0) {
      base[k] = Math.max(PANEL_HEIGHT_MIN_WEIGHT, v);
    }
  }
  return base;
}

function isLocale(v: unknown): v is Locale {
  return typeof v === 'string' && (SUPPORTED_LOCALES as string[]).includes(v);
}

// readStoredLocale mirrors readStoredTheme: a saved pick wins, otherwise we
// sniff navigator.language with a strict fallback to Spanish. The dedicated
// helper in i18n/index.ts uses the same algorithm — duplicating it here would
// risk drift, so we keep that as the source of truth and re-export.
export { readStoredLocale } from '../i18n';

// applyTheme writes the active theme to <html data-theme="..."> so the
// :root[data-theme] rules in style.css take effect. Exposed at module scope
// so main.ts can call it before Vue mounts, avoiding a brief flash of the
// default amber palette on top of a saved non-amber pick.
export function applyTheme(theme: Theme) {
  if (typeof document === 'undefined') return;
  document.documentElement.setAttribute('data-theme', theme);
}

export function readStoredTheme(): Theme {
  if (typeof window === 'undefined') return DEFAULTS.theme;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return DEFAULTS.theme;
    const parsed = JSON.parse(raw) as Partial<PrefsState>;
    const t = parsed.theme as string | undefined;
    // The "modern" theme was merged into "dark" — silently migrate any
    // pre-existing preference so returning users see the new (sharper) dark
    // instead of having their localStorage entry rejected as invalid.
    if (t === 'modern') return 'dark';
    if (t === 'amber' || t === 'green' || t === 'dark' || t === 'accesible') return t;
  } catch { /* corrupt entry → default */ }
  return DEFAULTS.theme;
}

// Bumped when the meaning of the right-column panel toggles changed. Storage
// written before this version may carry collapsed/height values that no
// longer correspond to the current layout; we discard those toggle-specific
// fields on load and start fresh from the all-expanded defaults. Other prefs
// (theme, locale, sidebar, mode, density, narrators) are kept regardless,
// since their schemas have not changed.
const PANELS_RESET_VERSION = 3;

function loadFromStorage(): PrefsState {
  if (typeof window === 'undefined') return DEFAULTS;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    // No stored prefs yet: keep DEFAULTS but resolve the locale via the same
    // detection the i18n module used at boot (navigator.language → 'en',
    // else 'es'). Without this the i18n instance would start in 'en' while
    // the store ref reads 'es', leaving the Preferences switch out of sync
    // with the actual UI language on the user's first visit.
    if (!raw) return { ...DEFAULTS, locale: i18nReadStoredLocale() };
    const parsed = JSON.parse(raw) as Partial<PrefsState> & { panelsResetVersion?: number };
    const upToDate = parsed.panelsResetVersion === PANELS_RESET_VERSION;
    return {
      ...DEFAULTS,
      ...parsed,
      // Nested objects need a deep merge so older storage payloads (missing
      // panelsCollapsed/columnWidths) don't blow away the defaults. If the
      // stored payload predates the toggle rework, reset panelsCollapsed and
      // panelHeights so the user never inherits an opaque "all collapsed"
      // state from a previous iteration.
      panelsCollapsed: upToDate
        ? sanitizePanelsCollapsed(parsed.panelsCollapsed)
        : { ...DEFAULTS.panelsCollapsed },
      panelHeights: upToDate
        ? sanitizePanelHeights(parsed.panelHeights)
        : { ...DEFAULTS.panelHeights },
      columnWidths: sanitizeColumnWidths(parsed.columnWidths),
    };
  } catch {
    return { ...DEFAULTS, locale: i18nReadStoredLocale() };
  }
}

export const usePreferencesStore = defineStore('preferences', () => {
  const initial = loadFromStorage();
  const sidebarCollapsed = ref(initial.sidebarCollapsed);
  const showInit = ref(initial.showInit);
  const showInitTouched = ref(initial.showInitTouched);
  const mode = ref<PresentationMode>(initial.mode);
  const density = ref<Density>(initial.density);
  const narratorsEnabled = ref(initial.narratorsEnabled);
  const narratorMinTickMs = ref(initial.narratorMinTickMs);
  const theme = ref<Theme>(initial.theme);
  const locale = ref<Locale>(isLocale(initial.locale) ? initial.locale : DEFAULT_LOCALE);
  const panelsCollapsed = ref<PanelsCollapsed>({ ...initial.panelsCollapsed });
  const columnWidths = ref<ColumnWidths>({ ...initial.columnWidths });
  const panelHeights = ref<PanelHeights>({ ...initial.panelHeights });

  // Mobile drawer state is volatile by design: always starts closed so a
  // fresh reload doesn't reveal the sidebar over the content. Kept separate
  // from sidebarCollapsed (desktop, persisted) so the two never fight.
  const sidebarMobileOpen = ref(false);

  // Keep the document attribute in sync with the store. main.ts already calls
  // applyTheme() before mount to avoid a flash, but if the theme changes at
  // runtime this watcher handles the swap without each component having to
  // observe the store itself.
  watch(theme, (t) => applyTheme(t), { immediate: false });

  function serialize(): string {
    return JSON.stringify({
      panelsResetVersion: PANELS_RESET_VERSION,
      sidebarCollapsed: sidebarCollapsed.value,
      showInit: showInit.value,
      showInitTouched: showInitTouched.value,
      mode: mode.value,
      density: density.value,
      narratorsEnabled: narratorsEnabled.value,
      narratorMinTickMs: narratorMinTickMs.value,
      theme: theme.value,
      locale: locale.value,
      panelsCollapsed: panelsCollapsed.value,
      columnWidths: columnWidths.value,
      panelHeights: panelHeights.value,
    });
  }

  watch(
    [sidebarCollapsed, showInit, showInitTouched, mode, density, narratorsEnabled, narratorMinTickMs, theme, locale, panelsCollapsed, columnWidths, panelHeights],
    () => {
      try {
        window.localStorage.setItem(STORAGE_KEY, serialize());
      } catch {
        // Quota or private mode — silently fall back to in-memory state.
      }
    },
    { deep: true },
  );

  // Auto-derived knobs that components consume directly. Keeping these as
  // computed refs means a change to `mode` propagates everywhere at once.
  const rowDensity = computed(() => (density.value === 'compact' ? 0.7 : 1));
  const animationsEnabled = computed(() => mode.value !== 'compact');
  const animationSpeed = computed(() => {
    if (mode.value === 'projector') return 0.5;
    if (mode.value === 'pedagogic') return 0.85;
    return 1;
  });

  function toggleSidebar() {
    sidebarCollapsed.value = !sidebarCollapsed.value;
  }

  function setShowInit(v: boolean) {
    showInit.value = v;
    // Flag the toggle as user-driven so later `loaded` events stop overwriting
    // the pick via applyShowInitDefault. This is persisted alongside showInit
    // so the override survives reloads.
    showInitTouched.value = true;
  }

  // applyShowInitDefault reveals init by default on every `loaded` event,
  // regardless of demo shape. Once the user has explicitly toggled
  // (showInitTouched=true) the default becomes a no-op and their pick wins
  // for the rest of the session.
  function applyShowInitDefault(_nonInitProcCount: number) {
    if (showInitTouched.value) return;
    showInit.value = true;
  }

  function togglePanel(name: PanelName, value?: boolean) {
    const next = typeof value === 'boolean' ? value : !panelsCollapsed.value[name];
    panelsCollapsed.value = { ...panelsCollapsed.value, [name]: next };
  }

  function setColumnWidth(side: ColumnSide, px: number) {
    // Clamp at the lower bound here; the upper bound depends on the current
    // viewport width and is enforced by useResizableColumn during drag and
    // on window resize, so this action just sanitizes the persisted value.
    const min = COLUMN_MIN_PX[side];
    const clamped = Math.max(min, Math.round(px));
    if (side === 'left') {
      if (columnWidths.value.leftPx === clamped) return;
      columnWidths.value = { ...columnWidths.value, leftPx: clamped };
    } else {
      if (columnWidths.value.rightPx === clamped) return;
      columnWidths.value = { ...columnWidths.value, rightPx: clamped };
    }
  }

  function setPanelHeight(name: StackPanelName, weight: number) {
    const clamped = Math.max(PANEL_HEIGHT_MIN_WEIGHT, weight);
    if (panelHeights.value[name] === clamped) return;
    panelHeights.value = { ...panelHeights.value, [name]: clamped };
  }

  // Layout snapshot covers everything the user can move/resize/collapse from
  // the simulator chrome: side column widths, vertical weights and the
  // collapsed state of each side panel. Theme, locale, mode, density and
  // narrator preferences are intentionally NOT part of "layout" — they are
  // editorial choices that survive a UI reset.
  function getLayoutSnapshot(): LayoutSnapshot {
    return {
      panelsCollapsed: { ...panelsCollapsed.value },
      columnWidths: { ...columnWidths.value },
      panelHeights: { ...panelHeights.value },
    };
  }

  function applyLayoutSnapshot(snap: LayoutSnapshot) {
    panelsCollapsed.value = { ...snap.panelsCollapsed };
    columnWidths.value = { ...snap.columnWidths };
    panelHeights.value = { ...snap.panelHeights };
  }

  function resetLayout() {
    applyLayoutSnapshot({
      panelsCollapsed: { ...DEFAULTS.panelsCollapsed },
      columnWidths: { ...DEFAULTS.columnWidths },
      panelHeights: { ...DEFAULTS.panelHeights },
    });
  }

  function openSidebarMobile() {
    sidebarMobileOpen.value = true;
  }

  function closeSidebarMobile() {
    sidebarMobileOpen.value = false;
  }

  function toggleSidebarMobile() {
    sidebarMobileOpen.value = !sidebarMobileOpen.value;
  }

  function setMode(m: PresentationMode) {
    mode.value = m;
    // Pedagogic / projector default to narrators on for first-time users; they
    // can still toggle narrators off explicitly afterwards.
    if (m !== 'compact') narratorsEnabled.value = true;
  }

  function setTheme(t: Theme) {
    theme.value = t;
  }

  // setLocale is the canonical mutation point for switching languages. It
  // updates the persisted ref AND the live vue-i18n instance AND <html lang>
  // in one explicit step — preferable to a watcher because the order is
  // obvious and there's no risk of stale renders. The fire-and-forget POST
  // to /api/i18n/lang persists the cookie the backend reads on the next
  // server-rendered page, so the locale survives full reloads end-to-end.
  function setLocale(l: Locale) {
    if (!isLocale(l)) return;
    locale.value = l;
    setI18nLocale(l);
    if (typeof window !== 'undefined') {
      void fetch('/api/i18n/lang', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ locale: l }),
      }).catch(() => undefined);
    }
  }

  // Keep <html lang> in sync on store creation. main.ts already called
  // applyLocale(readStoredLocale()) before mount, but if the store boots from
  // a different value (corrupt storage, defaults), this catches up.
  applyLocale(locale.value);

  return {
    sidebarCollapsed,
    sidebarMobileOpen,
    showInit,
    setShowInit,
    applyShowInitDefault,
    mode,
    density,
    narratorsEnabled,
    narratorMinTickMs,
    theme,
    locale,
    panelsCollapsed,
    columnWidths,
    panelHeights,
    rowDensity,
    animationsEnabled,
    animationSpeed,
    toggleSidebar,
    openSidebarMobile,
    closeSidebarMobile,
    toggleSidebarMobile,
    setMode,
    setTheme,
    setLocale,
    togglePanel,
    setColumnWidth,
    setPanelHeight,
    getLayoutSnapshot,
    applyLayoutSnapshot,
    resetLayout,
  };
});
