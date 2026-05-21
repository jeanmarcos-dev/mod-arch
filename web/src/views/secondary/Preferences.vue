<script setup lang="ts">
import Card from '../../components/ui/Card.vue';
import { usePreferencesStore, type PresentationMode, type Density, type Theme } from '../../stores/preferences';

const prefs = usePreferencesStore();

interface ModeOption {
  id: PresentationMode;
  label: string;
  desc: string;
}

const modes: ModeOption[] = [
  {
    id: 'compact',
    label: 'Compacto',
    desc: 'Densidad alta, paneles densos, sin animaciones extra. Recomendado para uso diario en una pantalla normal.',
  },
  {
    id: 'pedagogic',
    label: 'Pedagógico',
    desc: 'Paneles más grandes, narradores visuales activos, tooltips explicativos. Ideal para que el alumno entienda cada paso.',
  },
  {
    id: 'projector',
    label: 'Demo / Proyector',
    desc: 'Texto extra grande, animaciones lentas, pausa entre eventos. Para mostrar la simulación en clase con un cañón.',
  },
];

const densities: { id: Density; label: string; desc: string }[] = [
  { id: 'comfortable', label: 'Cómoda', desc: 'Espaciado generoso entre filas y paneles.' },
  { id: 'compact', label: 'Compacta', desc: 'Más información a la vez, espaciado mínimo.' },
];

interface ThemeOption {
  id: Theme;
  label: string;
  desc: string;
  swatch: string; // hex of the accent color, used to paint the mini preview
}

const themes: ThemeOption[] = [
  {
    id: 'amber',
    label: 'CRT ámbar',
    desc: 'Fósforo ámbar clásico. El tema por defecto, evocando las terminales VT220.',
    swatch: '#ffb547',
  },
  {
    id: 'green',
    label: 'CRT verde',
    desc: 'Fósforo verde estilo VT100. Misma estética CRT, paleta más fría.',
    swatch: '#7fff95',
  },
  {
    id: 'modern',
    label: 'Oscuro moderno',
    desc: 'Paleta azul-gris plana y tipografía sans-serif. Pensado para proyección diurna en aulas claras.',
    swatch: '#7fa4ff',
  },
];
</script>

<template>
  <div class="prefs">
    <Card title="Tema visual" :elevation="1" class="prefs__card">
      <p class="prefs__hint">
        El tema cambia la paleta del simulador completo: paneles, narradores,
        gráficos y la SimBar. Los dos temas CRT mantienen las tipografías
        retro; el oscuro moderno usa sans-serif para proyectar en clases con
        luz fuerte.
      </p>
      <div class="prefs__options">
        <button
          v-for="t in themes"
          :key="t.id"
          type="button"
          class="prefs__option prefs__option--theme"
          :class="{ 'prefs__option--on': prefs.theme === t.id }"
          @click="prefs.setTheme(t.id)"
        >
          <span class="prefs__theme-row">
            <span class="prefs__theme-swatch" :style="{ background: t.swatch }" aria-hidden="true"></span>
            <span class="prefs__option-label">{{ t.label }}</span>
          </span>
          <span class="prefs__option-desc">{{ t.desc }}</span>
        </button>
      </div>
    </Card>

    <Card title="Modo de presentación" :elevation="1" class="prefs__card">
      <p class="prefs__hint">
        Cambie el modo según el contexto en el que usa la herramienta. Los tres
        presets afectan tamaño de texto, densidad y animaciones de un solo
        golpe.
      </p>
      <div class="prefs__options">
        <button
          v-for="m in modes"
          :key="m.id"
          type="button"
          class="prefs__option"
          :class="{ 'prefs__option--on': prefs.mode === m.id }"
          @click="prefs.setMode(m.id)"
        >
          <span class="prefs__option-label">{{ m.label }}</span>
          <span class="prefs__option-desc">{{ m.desc }}</span>
        </button>
      </div>
    </Card>

    <Card title="Densidad de las tablas" :elevation="1" class="prefs__card">
      <div class="prefs__options prefs__options--row">
        <button
          v-for="d in densities"
          :key="d.id"
          type="button"
          class="prefs__option"
          :class="{ 'prefs__option--on': prefs.density === d.id }"
          @click="prefs.density = d.id"
        >
          <span class="prefs__option-label">{{ d.label }}</span>
          <span class="prefs__option-desc">{{ d.desc }}</span>
        </button>
      </div>
    </Card>

    <Card title="Narradores visuales" :elevation="1" class="prefs__card">
      <p class="prefs__hint">
        Activa overlays animados sobre los paneles del simulador: bytes
        viajando entre CPU y memoria, copia de PCB en cambio de contexto,
        cambios de modo USER ↔ KERNEL. Coste de rendimiento bajo a moderado.
      </p>
      <label class="prefs__toggle">
        <input
          type="checkbox"
          :checked="prefs.narratorsEnabled"
          @change="(ev) => (prefs.narratorsEnabled = (ev.target as HTMLInputElement).checked)"
        />
        <span>{{ prefs.narratorsEnabled ? 'Activados' : 'Desactivados' }}</span>
      </label>
    </Card>

    <Card title="Sidebar" :elevation="1" class="prefs__card">
      <label class="prefs__toggle">
        <input
          type="checkbox"
          :checked="prefs.sidebarCollapsed"
          @change="prefs.toggleSidebar()"
        />
        <span>Iniciar contraído (sólo íconos)</span>
      </label>
    </Card>
  </div>
</template>

<style scoped>
.prefs {
  flex: 1;
  padding: var(--space-5);
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  max-width: 880px;
  width: 100%;
  margin: 0 auto;
}

.prefs__card { width: 100%; }

.prefs__hint {
  color: var(--text-dim);
  font-size: 12px;
  line-height: 1.5;
  margin-bottom: var(--space-3);
}

.prefs__options {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--space-3);
}

.prefs__options--row {
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
}

.prefs__option {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text);
  cursor: pointer;
  padding: var(--space-3);
  text-align: left;
  font-family: inherit;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  border-radius: var(--radius-sm);
  transition:
    background var(--transition-fast),
    border-color var(--transition-fast),
    box-shadow var(--transition-fast),
    transform var(--transition-fast);
}

.prefs__option:hover {
  background: rgba(var(--accent-rgb), 0.06);
  border-color: var(--border-strong);
  transform: translateY(-1px);
}

.prefs__option--on {
  background: rgba(var(--accent-rgb), 0.14);
  border-color: var(--accent);
  box-shadow: var(--shadow-glow-amber);
}

.prefs__option-label {
  font-family: var(--font-display);
  font-size: 12px;
  letter-spacing: 2px;
  color: var(--accent);
}

.prefs__option-desc {
  font-size: 12px;
  color: var(--text-dim);
  line-height: 1.5;
}

.prefs__theme-row {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
}

.prefs__theme-swatch {
  width: 18px;
  height: 18px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border-strong);
  box-shadow: 0 0 8px rgba(0, 0, 0, 0.45);
  flex-shrink: 0;
}

.prefs__toggle {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  cursor: pointer;
  font-size: 13px;
  color: var(--text);
}

.prefs__toggle input {
  width: 16px;
  height: 16px;
  accent-color: var(--accent);
}
</style>
