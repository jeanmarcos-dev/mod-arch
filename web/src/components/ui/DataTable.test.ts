import { describe, it, expect } from 'vitest';
import { mount } from '@vue/test-utils';
import DataTable from './DataTable.vue';

interface User extends Record<string, unknown> {
  id: string;
  name: string;
  age: number;
}

const rows: User[] = [
  { id: '1', name: 'Ada', age: 30 },
  { id: '2', name: 'Linus', age: 22 },
  { id: '3', name: 'Grace', age: 85 },
];

const columns = [
  { id: 'name', label: 'Nombre' },
  { id: 'age', label: 'Edad' },
];

describe('DataTable', () => {
  it('renders one row per data item', () => {
    const wrapper = mount(DataTable, { props: { rows, columns } });
    expect(wrapper.findAll('tbody tr')).toHaveLength(3);
  });

  it('filters rows by the search box', async () => {
    const wrapper = mount(DataTable, { props: { rows, columns } });
    const input = wrapper.find('input[type="search"]');
    await input.setValue('linus');
    expect(wrapper.findAll('tbody tr')).toHaveLength(1);
    expect(wrapper.text()).toContain('Linus');
  });

  it('shows the search-specific empty state when there are no matches', async () => {
    const wrapper = mount(DataTable, { props: { rows, columns } });
    await wrapper.find('input[type="search"]').setValue('nadie');
    expect(wrapper.text()).toContain('Sin coincidencias');
  });

  it('sorts by clicking a column header', async () => {
    const wrapper = mount(DataTable, { props: { rows, columns } });
    // first click sorts ascending by age
    await wrapper.findAll('th')[1].trigger('click');
    let firstRowText = wrapper.findAll('tbody tr')[0].text();
    expect(firstRowText).toContain('Linus');
    // second click flips to descending
    await wrapper.findAll('th')[1].trigger('click');
    firstRowText = wrapper.findAll('tbody tr')[0].text();
    expect(firstRowText).toContain('Grace');
  });
});
