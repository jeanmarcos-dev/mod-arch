import { describe, it, expect } from 'vitest';
import { defineComponent, h } from 'vue';
import { mount } from '@vue/test-utils';
import { useNarrators, type LockOp } from './narratorBus';
import { useCpuStore, type CpuEvent } from '../stores/cpu';

function mountBus() {
  let bus!: ReturnType<typeof useNarrators>;
  const Host = defineComponent({
    setup() {
      bus = useNarrators();
      return () => h('div');
    },
  });
  const wrapper = mount(Host);
  const cpu = useCpuStore();
  return {
    bus,
    dispatch: (ev: CpuEvent) =>
      (cpu as unknown as { __applyEvent: (e: CpuEvent) => void }).__applyEvent(ev),
    unmount: () => wrapper.unmount(),
  };
}

describe('narratorBus lock channels', () => {
  it('fans out mutex_op events to onMutexOp with the normalized payload', () => {
    const { bus, dispatch } = mountBus();
    const got: LockOp[] = [];
    bus.onMutexOp(e => got.push(e));

    dispatch({
      type: 'mutex_op', core_id: 1, tick: 5, pid: 2,
      mutex_id: 1, op: 'handoff', next_pid: 3, waiters: [4],
    } as CpuEvent);

    expect(got.length).toBe(1);
    expect(got[0]).toMatchObject({
      op: 'handoff', lockId: 1, pid: 2, nextPid: 3, waiters: [4], coreId: 1,
    });
    expect(got[0].seq).toBeGreaterThan(0);
  });

  it('fans out sem_op events to onSemOp and defaults waiters to []', () => {
    const { bus, dispatch } = mountBus();
    const got: LockOp[] = [];
    bus.onSemOp(e => got.push(e));

    dispatch({
      type: 'sem_op', core_id: 0, tick: 1, pid: 1, sem_id: 2, op: 'signal', count: 1,
    } as CpuEvent);

    expect(got.length).toBe(1);
    expect(got[0]).toMatchObject({ op: 'signal', lockId: 2, pid: 1, waiters: [], coreId: 0 });
    expect(got[0].nextPid).toBeUndefined();
  });

  it('does not cross channels: mutex_op never reaches onSemOp', () => {
    const { bus, dispatch } = mountBus();
    const sem: LockOp[] = [];
    bus.onSemOp(e => sem.push(e));
    dispatch({
      type: 'mutex_op', core_id: 0, tick: 1, pid: 1, mutex_id: 1, op: 'acquired',
    } as CpuEvent);
    expect(sem.length).toBe(0);
  });
});
