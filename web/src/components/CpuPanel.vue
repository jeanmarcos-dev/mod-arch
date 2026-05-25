<script setup lang="ts">
import { useCpuStore, REGISTERS, type CoreInfo } from '../stores/cpu';
import { computed } from 'vue';

const cpu = useCpuStore();

function hex(n: number, w = 8) {
  return '0x' + (n >>> 0).toString(16).padStart(w, '0').toUpperCase();
}

// Tooltips por registro: explicación corta y al hueso.
const REG_HELP: Record<string, string> = {
  EAX: 'Accumulator. Registro de propósito general. En syscalls Linux: número del syscall en la entrada, resultado en la salida.',
  EBX: 'Base. Propósito general. En syscalls Linux i386: arg1.',
  ECX: 'Counter. Propósito general. En syscalls Linux i386: arg2. Históricamente usado por LOOP.',
  EDX: 'Data. Propósito general. En syscalls Linux i386: arg3. Usado por MUL/DIV para resultados de 64 bits.',
  ESI: 'Source Index. Usado por instrucciones de string (movs, lods).',
  EDI: 'Destination Index. Usado por instrucciones de string (movs, stos).',
  ESP: 'Stack Pointer. Apunta al tope de la pila del proceso. PUSH decrementa, POP incrementa.',
  EBP: 'Base Pointer. Apunta a la base del stack frame actual. Útil para acceder a parámetros y locales.',
  EIP: 'Instruction Pointer. Dirección de la próxima instrucción a ejecutar. Solo cambia por JMP/CALL/RET/INT/IRET o automáticamente tras fetch.',
  EFLAGS: 'Status register. Bits que reflejan el último resultado aritmético (ZF/SF/CF/OF) y configuración del CPU (IF, etc.).',
};

const SEG_HELP: Record<string, string> = {
  CS: 'Code Segment selector. Bits 0-1 = RPL (Requested Privilege Level). En user mode = 0x1B (0x18|3), en kernel = 0x08. IRET lo consulta para decidir el modo de retorno.',
  DS: 'Data Segment selector. Para accesos de datos (mov, push memoria, etc.).',
  SS: 'Stack Segment selector. Asociado al stack del proceso actual.',
};

function flagsString(eflags: number): string {
  return [
    eflags & 0x001 ? 'C' : '-',
    eflags & 0x040 ? 'Z' : '-',
    eflags & 0x080 ? 'S' : '-',
    eflags & 0x200 ? 'I' : '-',
    eflags & 0x800 ? 'O' : '-',
  ].join('');
}

const FLAGS_HELP = 'Flags activos: C=Carry (overflow no signado), Z=Zero (resultado fue 0), S=Sign (resultado negativo), I=Interrupt (IRQs habilitadas), O=Overflow (overflow signado). - = apagado.';
const PL_HELP = 'PL = Privilege Level. PL3 = USER (procesos comunes, sin permiso de tocar memoria/recursos kernel). PL0 = KERNEL (privilegios totales). El HW cambia el PL automáticamente en INT/IRET/excepción — el SW no puede setearlo directamente.';

// Cores currently visible in the SimBar filter, sorted by id for stable
// vertical order. Empty array falls back to a synthetic boot core so the
// panel never blanks while the snapshot is in flight.
const visibleCores = computed<CoreInfo[]>(() => {
  const filtered = cpu.cores.filter(c => cpu.isCoreVisible(c.id));
  if (filtered.length > 0) return filtered;
  return [{
    id: 0,
    regs: { ...cpu.regs },
    segments: { ...cpu.segments },
    mode: cpu.mode,
    halted: cpu.halted,
    tick: cpu.tick,
    current_pid: cpu.currentPID,
  }];
});

// changedRegsByCore stamps every reg_write into the right core's slot, so a
// write on C1 flashes the C1 strip even when C0 is the focused core. The
// legacy top-level changedRegs set is left in place for any external
// consumer, but the panel reads the per-core map directly.
function shouldFlash(core: CoreInfo, reg: string): boolean {
  return cpu.changedRegsByCore[core.id]?.has(reg) ?? false;
}
</script>

<template>
  <div
    class="panel cpu"
    data-pizarra-anchor="cpu-panel"
    data-narrator-anchor="cpu"
  >
    <div class="panel-title">CPU · REGISTROS</div>
    <section
      v-for="core in visibleCores"
      :key="`core-${core.id}`"
      :class="['cpu-core', { 'cpu-core--focus': core.id === cpu.selectedCoreId, 'cpu-core--halted': core.halted }]"
      :data-narrator-anchor="`cpu-${core.id}`"
    >
      <header v-if="visibleCores.length > 1" class="cpu-core__head">
        <span class="cpu-core__id">C{{ core.id }}</span>
        <span class="cpu-core__pid">{{ core.current_pid ? `P${core.current_pid}` : 'idle' }}</span>
        <span v-if="core.halted" class="cpu-core__tag">HALT</span>
      </header>
      <div
        class="reg-row clickable"
        v-for="r in REGISTERS"
        :key="r"
        :class="{ flash: shouldFlash(core, r) }"
        :title="REG_HELP[r] + '\n\n— click para abrir el glosario —'"
        @click="cpu.openGlossary(r)"
      >
        <span class="reg-name">{{ r }}</span>
        <span class="reg-val">{{ hex(core.regs[r] ?? 0) }}</span>
      </div>
      <div class="separator"></div>
      <div class="reg-row clickable" :title="FLAGS_HELP + '\n\n— click para abrir el glosario —'" @click="cpu.openGlossary('EFLAGS')">
        <span class="reg-name">FLAGS</span>
        <span class="reg-val">{{ flagsString(core.regs.EFLAGS ?? 0) }}</span>
      </div>
      <div class="reg-row clickable" :title="SEG_HELP.CS + '\n\n— click para abrir el glosario —'" @click="cpu.openGlossary('CS')">
        <span class="reg-name">CS</span>
        <span class="reg-val">{{ hex(core.segments.CS, 4) }}</span>
      </div>
      <div class="reg-row clickable" :title="SEG_HELP.DS + '\n\n— click para abrir el glosario —'" @click="cpu.openGlossary('DS')">
        <span class="reg-name">DS</span>
        <span class="reg-val">{{ hex(core.segments.DS, 4) }}</span>
      </div>
      <div class="reg-row clickable" :title="SEG_HELP.SS + '\n\n— click para abrir el glosario —'" @click="cpu.openGlossary('SS')">
        <span class="reg-name">SS</span>
        <span class="reg-val">{{ hex(core.segments.SS, 4) }}</span>
      </div>
      <div class="separator"></div>
      <div class="reg-row clickable" :title="PL_HELP + '\n\n— click para abrir el glosario —'" @click="cpu.openGlossary('PL')">
        <span class="reg-name">PL</span>
        <span class="reg-val" :style="{ color: core.mode === 3 ? 'var(--green)' : 'var(--red-soft)' }">
          {{ core.mode }} ({{ core.mode === 3 ? 'user' : 'kernel' }})
        </span>
      </div>
    </section>
  </div>
</template>

<style scoped>
.cpu {
  width: 200px;
  flex-shrink: 0;
  height: 100%;
  overflow-y: auto;
}

@media (max-width: 768px) {
  .cpu {
    width: 100%;
    height: auto;
    flex: 1;
  }

  .reg-row {
    padding: 8px 6px;
    font-size: 15px;
  }
}

.cpu-core {
  display: flex;
  flex-direction: column;
}
.cpu-core + .cpu-core {
  margin-top: 8px;
  padding-top: 6px;
  border-top: 1px dashed var(--border);
}
.cpu-core--halted {
  opacity: 0.85;
}

.cpu-core__head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 2px 4px 4px;
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  letter-spacing: 1px;
  border-bottom: 1px dashed var(--border);
  margin-bottom: 4px;
}
.cpu-core__id {
  color: var(--amber);
  font-weight: 600;
}
.cpu-core--focus .cpu-core__id {
  text-decoration: underline dotted var(--amber);
  text-underline-offset: 2px;
}
.cpu-core__pid {
  color: var(--text-dim);
}
.cpu-core__tag {
  margin-left: auto;
  font-size: 9px;
  padding: 0 4px;
  border: 1px solid var(--red-soft, #ff7a7a);
  color: var(--red-soft, #ff7a7a);
  border-radius: var(--radius-sm);
}

.reg-row {
  display: flex;
  justify-content: space-between;
  padding: 3px 4px;
  font-size: 14px;
}

.reg-row.clickable {
  cursor: pointer;
}

.reg-row.clickable:hover {
  background: rgba(106, 212, 255, 0.1);
  outline: 1px dashed var(--cyan);
}

.reg-name {
  color: var(--cyan);
  font-weight: 500;
}

.reg-val {
  color: var(--yellow);
  font-family: 'JetBrains Mono', monospace;
}

.separator {
  height: 1px;
  background: var(--border);
  margin: 8px 0;
}
</style>