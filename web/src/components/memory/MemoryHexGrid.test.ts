import { describe, it, expect, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import MemoryHexGrid, { type HeatEntry } from './MemoryHexGrid.vue';
import { i18n } from '../../i18n';

beforeEach(() => {
  (i18n.global.locale as unknown as { value: string }).value = 'es';
});

const BASE = 0x200000;
const ROWS = 4;

function mountGrid(disputedAddr: number | null) {
  return mount(MemoryHexGrid, {
    global: { plugins: [i18n] },
    props: {
      baseAddr: BASE,
      rowCount: ROWS,
      bytes: Array.from({ length: ROWS * 16 }, (_, i) => i & 0xff),
      heat: new Map<number, HeatEntry>(),
      lastReadByAddr: new Map<number, HeatEntry>(),
      lastWriteByAddr: new Map<number, HeatEntry>(),
      disputedAddr,
    },
  });
}

describe('MemoryHexGrid disputed cell', () => {
  it('marks the cell at disputedAddr with the --disputed class', () => {
    const target = BASE + 0x12; // row 1, column 2 — inside the rendered window
    const w = mountGrid(target);
    const disputed = w.findAll('.hexgrid__cell--disputed');
    expect(disputed.length).toBe(1);
    expect(disputed[0].attributes('data-mem-addr')).toBe(`0x${target.toString(16)}`);
  });

  it('marks no cell when disputedAddr is null', () => {
    const w = mountGrid(null);
    expect(w.findAll('.hexgrid__cell--disputed').length).toBe(0);
    expect(w.findAll('.hexgrid__cell').length).toBe(ROWS * 16);
  });
});
