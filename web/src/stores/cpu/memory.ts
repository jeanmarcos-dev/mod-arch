import { ref } from 'vue';
import type { SourceLine, VarMeta, VarSnapshot } from './types';

export function useCpuMemory() {
  const procSources = ref<Record<number, string>>({});
  const procSourceMaps = ref<Record<number, SourceLine[]>>({});
  const procVarMeta = ref<Record<number, VarMeta>>({});
  const procVars = ref<Record<number, VarSnapshot>>({});
  const pendingMemoryJump = ref<number | null>(null);

  return {
    procSources, procSourceMaps, procVarMeta, procVars, pendingMemoryJump
  };
}
