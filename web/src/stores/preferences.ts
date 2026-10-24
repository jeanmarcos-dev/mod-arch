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

export type TextScale = 'normal' | 'large' | 'xlarge';

export const TEXT_SCALE_FACTORS: Record<TextScale, number> = {
  normal: 1,
  large: 1.25,
  xlarge: 1.5,
};

function isTextScale(v: unknown): v is TextScale {
  return v === 'normal' || v === 'large' || v === 'xlarge';
}

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

// AsidesCollapsed tracks whether the entire side column (CPU+IPC on the left,
// Procesos+Colas on the right) is folded into a 28px vertical handle. Distinct
// from panelsCollapsed, which collapses individual panels to their headers.
export interface AsidesCollapsed {
  left: boolean;
  right: boolean;
}

// PanelHeights stores a flex-grow weight per resizable row. The right column
// stack (Procesos/Colas/IPC) uses three weights, and the center column splits
// vertical space between the tab pile (Código/Bitácora/Memoria) and the
// console below it via two extra weights. Drag of the horizontal handles
// rewrites the weights so the user can rebalance any pair. Cores lives alone
// in the left column, so no weight is tracked for it.
export type StackPanelName = 'cpu' | 'processes' | 'queues' | 'ipc' | 'middleTabs' | 'middleConsole';

export interface PanelHeights {
  cpu: number;
  processes: number;
  queues: number;
  ipc: number;
  middleTabs: number;
  middleConsole: number;
}

export interface LayoutSnapshot {
  panelsCollapsed: PanelsCollapsed;
  asidesCollapsed: AsidesCollapsed;
  columnWidths: ColumnWidths;
  panelHeights: PanelHeights;
}

export const PANEL_HEIGHT_MIN_WEIGHT = 0.15;

// Console drawer persistent state. The drawer is anchored at the bottom of
// the whole simulator shell; collapsed it shows only a centered pill, and
// expanded it pushes the rest of the layout up by `heightPx`. Both fields
// are persisted so a returning user finds the same configuration.
export interface ConsoleDrawerState {
  expanded: boolean;
  heightPx: number;
}

export const CONSOLE_DRAWER_MIN_PX = 200;
export const CONSOLE_DRAWER_DEFAULT_PX = 280;
// Capped at half the viewport: any taller and the simulator panels above
// lose more height than the drawer is worth.
export const CONSOLE_DRAWER_MAX_RATIO = 0.5;

export function consoleDrawerMaxPx(): number {
  if (typeof window === 'undefined') return 600;
  return Math.max(CONSOLE_DRAWER_MIN_PX, Math.floor(window.innerHeight * CONSOLE_DRAWER_MAX_RATIO));
}

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
  // schedulerAnimationEnabled controls the QueuesPanel ready-queue micro
  // animations: slide-in for arrivals (wake / new), slide-out for dispatches
  // and a permanent pulse on the head. Independent from narratorsEnabled so
  // a teacher can keep PCB-copy narration off but still surface the queue
  // flow, or vice versa. Off by default in compact mode.
  schedulerAnimationEnabled: boolean;
  // Below this tick interval (ms) narrators stop firing. Pedagogically:
  // visual narration of every event is meaningless once the CPU ticks faster
  // than the eye can resolve. Configurable so the teacher can widen or
  // narrow the readable band.
  narratorMinTickMs: number;
  // clearLogOnLoad wipes the event log + console on load so each demo starts
  // clean. Off keeps history (the run-epoch stops ticks merging). Reset always
  // clears.
  clearLogOnLoad: boolean;
  // autoPauseOnIdle stops the run loop once no user process remains. Off keeps
  // the realistic "the CPU never stops" behavior.
  autoPauseOnIdle: boolean;
  theme: Theme;
  legibleFont: boolean;
  textScale: TextScale;
  screenReaderAnnounce: boolean;
  locale: Locale;
  panelsCollapsed: PanelsCollapsed;
  asidesCollapsed: AsidesCollapsed;
  columnWidths: ColumnWidths;
  panelHeights: PanelHeights;
  consoleDrawer: ConsoleDrawerState;
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
  schedulerAnimationEnabled: true,
  narratorMinTickMs: 200,
  clearLogOnLoad: true,
  autoPauseOnIdle: true,
  theme: 'amber',
  legibleFont: false,
  textScale: 'normal',
  screenReaderAnnounce: true,
  locale: DEFAULT_LOCALE,
  panelsCollapsed: {
    cores: false,
    processes: false,
    queues: false,
    ipc: false,
  },
  asidesCollapsed: {
    left: false,
    right: false,
  },
  columnWidths: {
    leftPx: 280,
    rightPx: 400,
  },
  panelHeights: {
    cpu: 1.4,
    processes: 1,
    queues: 1.2,
    ipc: 1,
    // Legacy weights kept so previously-saved payloads deserialize without
    // discarding everything; the console drawer no longer splits the center
    // column, so these are dead fields. Reading them is a no-op now.
    middleTabs: 1,
    middleConsole: 1.2,
  },
  consoleDrawer: {
    // Collapsed by default: a first-time visitor sees the full code +
    // bitácora area; the pill at the bottom signals when there is I/O worth
    // looking at. Returning users get whatever state they last left.
    expanded: false,
    heightPx: CONSOLE_DRAWER_DEFAULT_PX,
  },
};

// AdminDefaults is the admin-curated baseline a fresh visitor inherits. It is
// delivered by GET /api/config and applied per-field ONLY where the user has
// not made their own pick (see PrefsTouched / applyAdminDefaults). `speed` is
// the simulated clock in Hz; it lives in the cpu store, so applyAdminDefaults
// ignores it and the boot orchestrator forwards it to cpu.applyDefaultClockHz.
export interface AdminDefaults {
  narratorsEnabled: boolean;
  mode: PresentationMode;
  theme: Theme;
  density: Density;
  speed: number;
  narratorMinTickMs: number;
}

// PrefsTouched records which editorial prefs the user changed by hand. Once a
// field is touched its local value wins and admin defaults stop applying to it
// — the sticky contract. Generalizes the showInitTouched pattern to a map so
// every defaultable field is tracked uniformly.
export type TouchableField = 'narratorsEnabled' | 'mode' | 'theme' | 'density' | 'narratorMinTickMs';
export type PrefsTouched = Record<TouchableField, boolean>;

function freshTouched(): PrefsTouched {
  return { narratorsEnabled: false, mode: false, theme: false, density: false, narratorMinTickMs: false };
}

function sanitizePanelsCollapsed(input: unknown): PanelsCollapsed {
  const base: PanelsCollapsed = { ...DEFAULTS.panelsCollapsed };
  if (!input || typeof input !== 'object') return base;
  const src = input as Record<string, unknown>;
  for (const k of Object.keys(base) as PanelName[]) {
    if (typeof src[k] === 'boolean') base[k] = src[k] as boolean;
  }
  return base;
}

function sanitizeAsidesCollapsed(input: unknown): AsidesCollapsed {
  const base: AsidesCollapsed = { ...DEFAULTS.asidesCollapsed };
  if (!input || typeof input !== 'object') return base;
  const src = input as Record<string, unknown>;
  if (typeof src.left === 'boolean') base.left = src.left;
  if (typeof src.right === 'boolean') base.right = src.right;
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

function sanitizeConsoleDrawer(input: unknown): ConsoleDrawerState {
  const base: ConsoleDrawerState = { ...DEFAULTS.consoleDrawer };
  if (!input || typeof input !== 'object') return base;
  const src = input as Record<string, unknown>;
  if (typeof src.expanded === 'boolean') base.expanded = src.expanded;
  if (typeof src.heightPx === 'number' && Number.isFinite(src.heightPx)) {
    const maxPx = consoleDrawerMaxPx();
    base.heightPx = Math.min(Math.max(CONSOLE_DRAWER_MIN_PX, Math.round(src.heightPx)), maxPx);
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

export function applyLegibleFont(on: boolean) {
  if (typeof document === 'undefined') return;
  document.documentElement.setAttribute('data-legible-font', on ? 'true' : 'false');
}

export function applyTextScale(scale: TextScale) {
  if (typeof document === 'undefined') return;
  document.documentElement.setAttribute('data-text-scale', scale);
}

export function detectA11yPreferences(): Partial<Pick<PrefsState, 'theme' | 'legibleFont'>> {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return {};
  try {
    if (window.matchMedia('(prefers-contrast: more)').matches) {
      return { theme: 'accesible', legibleFont: true };
    }
  } catch { /* unsupported query */ }
  return {};
}

export function readStoredTheme(): Theme {
  if (typeof window === 'undefined') return DEFAULTS.theme;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return detectA11yPreferences().theme ?? DEFAULTS.theme;
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

export function readStoredLegibleFont(): boolean {
  if (typeof window === 'undefined') return DEFAULTS.legibleFont;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return detectA11yPreferences().legibleFont ?? DEFAULTS.legibleFont;
    const parsed = JSON.parse(raw) as Partial<PrefsState>;
    if (typeof parsed.legibleFont === 'boolean') return parsed.legibleFont;
  } catch { /* corrupt entry → default */ }
  return DEFAULTS.legibleFont;
}

export function readStoredTextScale(): TextScale {
  if (typeof window === 'undefined') return DEFAULTS.textScale;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return DEFAULTS.textScale;
    const parsed = JSON.parse(raw) as Partial<PrefsState>;
    if (isTextScale(parsed.textScale)) return parsed.textScale;
  } catch { /* corrupt entry → default */ }
  return DEFAULTS.textScale;
}

// Bumped when the meaning of the right-column panel toggles changed. Storage
// written before this version may carry collapsed/height values that no
// longer correspond to the current layout; we discard those toggle-specific
// fields on load and start fresh from the all-expanded defaults. Other prefs
// (theme, locale, sidebar, mode, density, narrators) are kept regardless,
// since their schemas have not changed.
const PANELS_RESET_VERSION = 4;

type LoadedPrefs = PrefsState & { touched: PrefsTouched };

// migrateTouched resolves the sticky-tracking map. A stored payload that
// already carries an explicit `touched` map is sanitized field-by-field.
// Legacy payloads (written before sticky defaults existed) have no map, so we
// infer it: a stored value that diverges from the code default is the best
// available signal of a conscious pick, and is treated as touched to shield it
// from admin defaults. Values equal to the default stay untouched and remain
// eligible to receive the admin's curated baseline.
function migrateTouched(parsed: Partial<PrefsState> & { touched?: unknown }, state: PrefsState): PrefsTouched {
  const raw = parsed.touched;
  if (raw && typeof raw === 'object') {
    const base = freshTouched();
    const src = raw as Record<string, unknown>;
    for (const k of Object.keys(base) as TouchableField[]) {
      if (typeof src[k] === 'boolean') base[k] = src[k] as boolean;
    }
    return base;
  }
  return {
    narratorsEnabled: state.narratorsEnabled !== DEFAULTS.narratorsEnabled,
    mode: state.mode !== DEFAULTS.mode,
    theme: state.theme !== DEFAULTS.theme,
    density: state.density !== DEFAULTS.density,
    narratorMinTickMs: state.narratorMinTickMs !== DEFAULTS.narratorMinTickMs,
  };
}

function loadFromStorage(): LoadedPrefs {
  if (typeof window === 'undefined') return { ...DEFAULTS, touched: freshTouched() };
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    // No stored prefs yet: keep DEFAULTS but resolve the locale via the same
    // detection the i18n module used at boot (navigator.language → 'en',
    // else 'es'). Without this the i18n instance would start in 'en' while
    // the store ref reads 'es', leaving the Preferences switch out of sync
    // with the actual UI language on the user's first visit. A fresh visitor
    // has touched nothing, so every field is eligible for admin defaults.
    if (!raw) return { ...DEFAULTS, ...detectA11yPreferences(), locale: i18nReadStoredLocale(), touched: freshTouched() };
    const parsed = JSON.parse(raw) as Partial<PrefsState> & { panelsResetVersion?: number; touched?: unknown };
    const upToDate = parsed.panelsResetVersion === PANELS_RESET_VERSION;
    const state: PrefsState = {
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
      asidesCollapsed: upToDate
        ? sanitizeAsidesCollapsed(parsed.asidesCollapsed)
        : { ...DEFAULTS.asidesCollapsed },
      panelHeights: upToDate
        ? sanitizePanelHeights(parsed.panelHeights)
        : { ...DEFAULTS.panelHeights },
      columnWidths: sanitizeColumnWidths(parsed.columnWidths),
      consoleDrawer: sanitizeConsoleDrawer(parsed.consoleDrawer),
    };
    return { ...state, touched: migrateTouched(parsed, state) };
  } catch {
    return { ...DEFAULTS, locale: i18nReadStoredLocale(), touched: freshTouched() };
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
  // sanitizePanelHeights et al. silently fall back to DEFAULTS on a missing
  // field, but the legacy storage shape predates schedulerAnimationEnabled so
  // a strict `?? DEFAULTS` keeps the new flag on for returning users.
  const schedulerAnimationEnabled = ref(initial.schedulerAnimationEnabled ?? DEFAULTS.schedulerAnimationEnabled);
  const narratorMinTickMs = ref(initial.narratorMinTickMs);
  const clearLogOnLoad = ref(initial.clearLogOnLoad ?? DEFAULTS.clearLogOnLoad);
  const autoPauseOnIdle = ref(initial.autoPauseOnIdle ?? DEFAULTS.autoPauseOnIdle);
  const theme = ref<Theme>(initial.theme);
  const legibleFont = ref(initial.legibleFont ?? DEFAULTS.legibleFont);
  const textScale = ref<TextScale>(isTextScale(initial.textScale) ? initial.textScale : DEFAULTS.textScale);
  const screenReaderAnnounce = ref(initial.screenReaderAnnounce ?? DEFAULTS.screenReaderAnnounce);
  const locale = ref<Locale>(isLocale(initial.locale) ? initial.locale : DEFAULT_LOCALE);
  const panelsCollapsed = ref<PanelsCollapsed>({ ...initial.panelsCollapsed });
  const asidesCollapsed = ref<AsidesCollapsed>({ ...initial.asidesCollapsed });
  const columnWidths = ref<ColumnWidths>({ ...initial.columnWidths });
  const panelHeights = ref<PanelHeights>({ ...initial.panelHeights });
  const consoleDrawer = ref<ConsoleDrawerState>({ ...initial.consoleDrawer });
  const touched = ref<PrefsTouched>({ ...initial.touched });

  // Mobile drawer state is volatile by design: always starts closed so a
  // fresh reload doesn't reveal the sidebar over the content. Kept separate
  // from sidebarCollapsed (desktop, persisted) so the two never fight.
  const sidebarMobileOpen = ref(false);

  // Presenter mirror: a viewer in autofollow reflects the host's presentation
  // mode and narrators flag WITHOUT touching their own persisted preference.
  // These refs are volatile (never serialized, never in the persistence watch)
  // so the spectator's local choice survives a reset or refresh — the host's
  // broadcast only drives the live view, not localStorage. Consumers read the
  // effective* computeds, which fall back to the local pref when no mirror is
  // active (owners and standalone students never activate it).
  const mirrorActive = ref(false);
  const mirrorMode = ref<PresentationMode | null>(null);
  const mirrorNarrators = ref<boolean | null>(null);

  const effectiveMode = computed<PresentationMode>(() =>
    mirrorActive.value && mirrorMode.value !== null ? mirrorMode.value : mode.value,
  );
  const effectiveNarratorsEnabled = computed<boolean>(() =>
    mirrorActive.value && mirrorNarrators.value !== null ? mirrorNarrators.value : narratorsEnabled.value,
  );

  // Keep the document attribute in sync with the store. main.ts already calls
  // applyTheme() before mount to avoid a flash, but if the theme changes at
  // runtime this watcher handles the swap without each component having to
  // observe the store itself.
  watch(theme, (t) => applyTheme(t), { immediate: false });
  watch(legibleFont, (v) => applyLegibleFont(v), { immediate: false });
  watch(textScale, (s) => applyTextScale(s), { immediate: false });

  function serialize(): string {
    return JSON.stringify({
      panelsResetVersion: PANELS_RESET_VERSION,
      sidebarCollapsed: sidebarCollapsed.value,
      showInit: showInit.value,
      showInitTouched: showInitTouched.value,
      mode: mode.value,
      density: density.value,
      narratorsEnabled: narratorsEnabled.value,
      schedulerAnimationEnabled: schedulerAnimationEnabled.value,
      narratorMinTickMs: narratorMinTickMs.value,
      clearLogOnLoad: clearLogOnLoad.value,
      autoPauseOnIdle: autoPauseOnIdle.value,
      theme: theme.value,
      legibleFont: legibleFont.value,
      textScale: textScale.value,
      screenReaderAnnounce: screenReaderAnnounce.value,
      locale: locale.value,
      panelsCollapsed: panelsCollapsed.value,
      asidesCollapsed: asidesCollapsed.value,
      columnWidths: columnWidths.value,
      panelHeights: panelHeights.value,
      consoleDrawer: consoleDrawer.value,
      touched: touched.value,
    });
  }

  watch(
    [sidebarCollapsed, showInit, showInitTouched, mode, density, narratorsEnabled, schedulerAnimationEnabled, narratorMinTickMs, clearLogOnLoad, autoPauseOnIdle, theme, legibleFont, textScale, screenReaderAnnounce, locale, panelsCollapsed, asidesCollapsed, columnWidths, panelHeights, consoleDrawer, touched],
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

  function setShowInit(v: boolean, fromCoupling = false) {
    showInit.value = v;
    // Flag the toggle as user-driven so later `loaded` events stop overwriting
    // the pick via applyShowInitDefault. This is persisted alongside showInit
    // so the override survives reloads. Coupled flips (e.g. the idle-task
    // toggle revealing init) pass fromCoupling=true so the user's own
    // preference is preserved if they later change their mind.
    if (!fromCoupling) showInitTouched.value = true;
  }

  // applyShowInitDefault reveals init by default on every `loaded` event,
  // regardless of demo shape. Once the user has explicitly toggled
  // (showInitTouched=true) the default becomes a no-op and their pick wins
  // for the rest of the session.
  function applyShowInitDefault(_nonInitProcCount: number) {
    if (showInitTouched.value) return;
    showInit.value = true;
  }

  function setClearLogOnLoad(v: boolean) {
    clearLogOnLoad.value = v;
  }

  function setAutoPauseOnIdle(v: boolean) {
    autoPauseOnIdle.value = v;
  }

  function togglePanel(name: PanelName, value?: boolean) {
    const next = typeof value === 'boolean' ? value : !panelsCollapsed.value[name];
    panelsCollapsed.value = { ...panelsCollapsed.value, [name]: next };
  }

  function toggleAside(side: ColumnSide, value?: boolean) {
    const next = typeof value === 'boolean' ? value : !asidesCollapsed.value[side];
    asidesCollapsed.value = { ...asidesCollapsed.value, [side]: next };
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

  function setConsoleDrawerExpanded(v: boolean) {
    if (consoleDrawer.value.expanded === v) return;
    consoleDrawer.value = { ...consoleDrawer.value, expanded: v };
  }

  function toggleConsoleDrawer() {
    consoleDrawer.value = { ...consoleDrawer.value, expanded: !consoleDrawer.value.expanded };
  }

  function setConsoleDrawerHeight(px: number) {
    const maxPx = consoleDrawerMaxPx();
    const clamped = Math.min(Math.max(CONSOLE_DRAWER_MIN_PX, Math.round(px)), maxPx);
    if (consoleDrawer.value.heightPx === clamped) return;
    consoleDrawer.value = { ...consoleDrawer.value, heightPx: clamped };
  }

  // Layout snapshot covers everything the user can move/resize/collapse from
  // the simulator chrome: side column widths, vertical weights and the
  // collapsed state of each side panel. Theme, locale, mode, density and
  // narrator preferences are intentionally NOT part of "layout" — they are
  // editorial choices that survive a UI reset.
  function getLayoutSnapshot(): LayoutSnapshot {
    return {
      panelsCollapsed: { ...panelsCollapsed.value },
      asidesCollapsed: { ...asidesCollapsed.value },
      columnWidths: { ...columnWidths.value },
      panelHeights: { ...panelHeights.value },
    };
  }

  function applyLayoutSnapshot(snap: LayoutSnapshot) {
    panelsCollapsed.value = { ...snap.panelsCollapsed };
    asidesCollapsed.value = { ...snap.asidesCollapsed };
    columnWidths.value = { ...snap.columnWidths };
    panelHeights.value = { ...snap.panelHeights };
  }

  function resetLayout() {
    applyLayoutSnapshot({
      panelsCollapsed: { ...DEFAULTS.panelsCollapsed },
      asidesCollapsed: { ...DEFAULTS.asidesCollapsed },
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

  function markTouched(field: TouchableField) {
    if (touched.value[field]) return;
    touched.value = { ...touched.value, [field]: true };
  }

  function setMode(m: PresentationMode) {
    mode.value = m;
    // Pedagogic / projector default to narrators on for first-time users; they
    // can still toggle narrators off explicitly afterwards. Scheduler
    // animation follows the same rule: in compact mode it goes off so the
    // ready queue stops shuffling chips around; in the richer modes it goes
    // back on to surface the wake/dispatch flow.
    if (m !== 'compact') {
      narratorsEnabled.value = true;
      schedulerAnimationEnabled.value = true;
    } else {
      schedulerAnimationEnabled.value = false;
    }
    // The coupling above flips narrators alongside the mode, so a manual mode
    // change counts as touching both — admin defaults must not later override
    // either against the user's deliberate pick.
    markTouched('mode');
    markTouched('narratorsEnabled');
  }

  function setTheme(t: Theme) {
    theme.value = t;
    markTouched('theme');
  }

  function setLegibleFont(v: boolean) {
    legibleFont.value = v;
  }

  function setTextScale(s: TextScale) {
    textScale.value = s;
  }

  function setScreenReaderAnnounce(v: boolean) {
    screenReaderAnnounce.value = v;
  }

  function setNarratorsEnabled(v: boolean) {
    narratorsEnabled.value = v;
    markTouched('narratorsEnabled');
  }

  function setDensity(d: Density) {
    density.value = d;
    markTouched('density');
  }

  function setNarratorMinTickMs(ms: number) {
    narratorMinTickMs.value = ms;
    markTouched('narratorMinTickMs');
  }

  // setMirror reflects the host's presentation into the volatile mirror layer.
  // Called by the viewer shell on each presence broadcast while autofollowing;
  // it never touches the persisted preference, so the spectator's own choice is
  // intact the moment they stop following (clearMirror) or reload.
  function setMirror(p: { mode?: string | null; narrators?: boolean | null }) {
    if (p.mode === 'compact' || p.mode === 'pedagogic' || p.mode === 'projector') {
      mirrorMode.value = p.mode;
    }
    if (typeof p.narrators === 'boolean') mirrorNarrators.value = p.narrators;
    mirrorActive.value = true;
  }

  function clearMirror() {
    mirrorActive.value = false;
    mirrorMode.value = null;
    mirrorNarrators.value = null;
  }

  // applyAdminDefaults writes the admin-curated baseline onto every field the
  // user has NOT touched. It assigns the refs directly (not via setMode/setTheme)
  // so it neither fires the mode→narrators coupling nor marks the field touched
  // — keeping those fields eligible if the admin later revises the default.
  function applyAdminDefaults(d: Partial<AdminDefaults> | null | undefined) {
    if (!d) return;
    if (!touched.value.narratorsEnabled && typeof d.narratorsEnabled === 'boolean') {
      narratorsEnabled.value = d.narratorsEnabled;
    }
    if (!touched.value.mode && (d.mode === 'compact' || d.mode === 'pedagogic' || d.mode === 'projector')) {
      mode.value = d.mode;
    }
    if (!touched.value.theme && (d.theme === 'amber' || d.theme === 'green' || d.theme === 'dark' || d.theme === 'accesible')) {
      theme.value = d.theme;
    }
    if (!touched.value.density && (d.density === 'comfortable' || d.density === 'compact')) {
      density.value = d.density;
    }
    if (!touched.value.narratorMinTickMs && typeof d.narratorMinTickMs === 'number'
      && Number.isFinite(d.narratorMinTickMs) && d.narratorMinTickMs >= 0) {
      narratorMinTickMs.value = d.narratorMinTickMs;
    }
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
    showInitTouched,
    setShowInit,
    applyShowInitDefault,
    mode,
    density,
    narratorsEnabled,
    schedulerAnimationEnabled,
    narratorMinTickMs,
    clearLogOnLoad,
    autoPauseOnIdle,
    setClearLogOnLoad,
    setAutoPauseOnIdle,
    theme,
    legibleFont,
    textScale,
    screenReaderAnnounce,
    setLegibleFont,
    setTextScale,
    setScreenReaderAnnounce,
    locale,
    touched,
    effectiveMode,
    effectiveNarratorsEnabled,
    setNarratorsEnabled,
    setDensity,
    setNarratorMinTickMs,
    setMirror,
    clearMirror,
    applyAdminDefaults,
    panelsCollapsed,
    asidesCollapsed,
    columnWidths,
    panelHeights,
    consoleDrawer,
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
    toggleAside,
    setColumnWidth,
    setPanelHeight,
    setConsoleDrawerExpanded,
    toggleConsoleDrawer,
    setConsoleDrawerHeight,
    getLayoutSnapshot,
    applyLayoutSnapshot,
    resetLayout,
  };
});
