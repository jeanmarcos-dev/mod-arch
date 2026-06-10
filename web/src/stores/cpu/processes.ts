import { ref } from 'vue';
import type { PCBInfo, WaitInfo, MutexState, SemaphoreState, PipeInfo } from './types';

export function useCpuProcesses() {
  const procs = ref<PCBInfo[]>([]);
  const readyQueue = ref<number[]>([]);
  const waitQueue = ref<WaitInfo[]>([]);
  const mutexes = ref<MutexState[]>([]);
  const semaphores = ref<SemaphoreState[]>([]);
  const pipes = ref<PipeInfo[]>([]);

  return {
    procs, readyQueue, waitQueue, mutexes, semaphores, pipes
  };
}
