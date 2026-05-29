import { computed, type ComputedRef } from 'vue';
import { useCpuStore, type PCBInfo } from '../stores/cpu';
import { usePreferencesStore } from '../stores/preferences';

export interface ProcTreeNode {
  proc: PCBInfo;
  depth: number;
  realDepth: number;
  isThread: boolean;
}

export interface UseProcessTreeOptions {
  depthCap?: number;
}

export interface UseProcessTreeResult {
  treeNodes: ComputedRef<ProcTreeNode[]>;
  initPID: ComputedRef<number>;
  treeParentOf: (p: PCBInfo) => number;
  isThreadOf: (p: PCBInfo) => boolean;
}

// Threads (tgid != pid) collapse under their leader; processes fall back to
// parent_pid so the tree stays hierarchical in both fork-only and threads
// demos.
function treeParentOf(p: PCBInfo): number {
  const tgid = p.tgid ?? p.pid;
  if (tgid !== p.pid) return tgid;
  return p.parent_pid ?? 0;
}

function isThreadOf(p: PCBInfo): boolean {
  const tgid = p.tgid ?? p.pid;
  return tgid !== p.pid;
}

export function useProcessTree(opts: UseProcessTreeOptions = {}): UseProcessTreeResult {
  const cpu = useCpuStore();
  const prefs = usePreferencesStore();
  const cap = opts.depthCap ?? Number.POSITIVE_INFINITY;

  const initPID = computed(() => {
    const p = cpu.procs.find(x => x.name === 'init');
    return p?.pid ?? 0;
  });

  const treeNodes = computed<ProcTreeNode[]>(() => {
    const hideInit = !prefs.showInit && initPID.value > 0;
    const visibleProcs = hideInit
      ? cpu.procs.filter(p => p.pid !== initPID.value)
      : cpu.procs;
    const byParent = new Map<number, PCBInfo[]>();
    for (const p of visibleProcs) {
      let pp = treeParentOf(p);
      // When init is hidden, promote its direct children to top-level so
      // they appear as roots instead of orphaned under a missing node.
      if (hideInit && pp === initPID.value) pp = 0;
      if (!byParent.has(pp)) byParent.set(pp, []);
      byParent.get(pp)!.push(p);
    }
    for (const list of byParent.values()) list.sort((a, b) => a.pid - b.pid);
    const out: ProcTreeNode[] = [];
    const visit = (parent: number, depth: number) => {
      const kids = byParent.get(parent);
      if (!kids) return;
      for (const k of kids) {
        out.push({
          proc: k,
          depth: Math.min(depth, cap),
          realDepth: depth,
          isThread: isThreadOf(k),
        });
        visit(k.pid, depth + 1);
      }
    };
    visit(0, 0);
    // Fallback for snapshots arriving mid-reload where some proc may lack a
    // resolvable parent — append them so they remain visible.
    if (out.length < visibleProcs.length) {
      const seen = new Set(out.map(n => n.proc.pid));
      for (const p of visibleProcs) {
        if (!seen.has(p.pid)) {
          out.push({ proc: p, depth: 0, realDepth: 0, isThread: isThreadOf(p) });
        }
      }
    }
    return out;
  });

  return { treeNodes, initPID, treeParentOf, isThreadOf };
}
