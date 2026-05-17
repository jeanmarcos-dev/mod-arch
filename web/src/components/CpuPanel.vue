<script setup lang="ts">
import { useCpuStore, REGISTERS } from '../stores/cpu';
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

const flagsString = computed(() => {
  const f = cpu.regs.EFLAGS ?? 0;
  return [
    f & 0x001 ? 'C' : '-',
    f & 0x040 ? 'Z' : '-',
    f & 0x080 ? 'S' : '-',
    f & 0x200 ? 'I' : '-',
    f & 0x800 ? 'O' : '-',
  ].join('');
});

const FLAGS_HELP = 'Flags activos: C=Carry (overflow no signado), Z=Zero (resultado fue 0), S=Sign (resultado negativo), I=Interrupt (IRQs habilitadas), O=Overflow (overflow signado). - = apagado.';

const PL_HELP = 'PL = Privilege Level. PL3 = USER (procesos comunes, sin permiso de tocar memoria/recursos kernel). PL0 = KERNEL (privilegios totales). El HW cambia el PL automáticamente en INT/IRET/excepción — el SW no puede setearlo directamente.';
</script>

<template>
  <div class="panel cpu">
    <div class="panel-title">CPU · REGISTROS</div>
    <div
      class="reg-row clickable"
      v-for="r in REGISTERS"
      :key="r"
      :class="{ flash: cpu.changedRegs.has(r) }"
      :title="REG_HELP[r] + '\n\n— click para abrir el glosario —'"
      @click="cpu.openGlossary(r)"
    >
      <span class="reg-name">{{ r }}</span>
      <span class="reg-val">{{ hex(cpu.regs[r] ?? 0) }}</span>
    </div>
    <div class="separator"></div>
    <div class="reg-row clickable" :title="FLAGS_HELP + '\n\n— click para abrir el glosario —'" @click="cpu.openGlossary('EFLAGS')">
      <span class="reg-name">FLAGS</span>
      <span class="reg-val">{{ flagsString }}</span>
    </div>
    <div class="reg-row clickable" :title="SEG_HELP.CS + '\n\n— click para abrir el glosario —'" @click="cpu.openGlossary('CS')">
      <span class="reg-name">CS</span>
      <span class="reg-val">{{ hex(cpu.segments.CS, 4) }}</span>
    </div>
    <div class="reg-row clickable" :title="SEG_HELP.DS + '\n\n— click para abrir el glosario —'" @click="cpu.openGlossary('DS')">
      <span class="reg-name">DS</span>
      <span class="reg-val">{{ hex(cpu.segments.DS, 4) }}</span>
    </div>
    <div class="reg-row clickable" :title="SEG_HELP.SS + '\n\n— click para abrir el glosario —'" @click="cpu.openGlossary('SS')">
      <span class="reg-name">SS</span>
      <span class="reg-val">{{ hex(cpu.segments.SS, 4) }}</span>
    </div>
    <div class="separator"></div>
    <div class="reg-row clickable" :title="PL_HELP + '\n\n— click para abrir el glosario —'" @click="cpu.openGlossary('PL')">
      <span class="reg-name">PL</span>
      <span class="reg-val" :style="{ color: cpu.mode === 3 ? 'var(--green)' : 'var(--red-soft)' }">
        {{ cpu.mode }} ({{ cpu.mode === 3 ? 'user' : 'kernel' }})
      </span>
    </div>
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
