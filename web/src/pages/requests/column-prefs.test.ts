import { beforeEach, describe, expect, it } from 'vitest';
import {
  DEFAULT_COLUMN_ORDER,
  DEFAULT_COLUMN_VISIBILITY,
  DEFAULT_COLUMN_WIDTHS,
  createDefaultColumnPrefs,
  migrateColumnPrefs,
  normalizeColumnPrefs,
  readColumnPrefs,
  resolveVisibleColumns,
  writeColumnPrefs,
} from './column-prefs';

describe('column-prefs', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('returns defaults for empty input', () => {
    const prefs = normalizeColumnPrefs(null);
    expect(prefs.order).toEqual([...DEFAULT_COLUMN_ORDER]);
    expect(prefs.widths.model).toBe(DEFAULT_COLUMN_WIDTHS.model);
    expect(prefs.visibility.protocol).toBe(true);
  });

  it('appends missing columns and drops unknown ids', () => {
    const prefs = normalizeColumnPrefs({
      order: ['model', 'unknown', 'time'],
      widths: { model: 160 },
      visibility: { model: false },
    });
    expect(prefs.order[0]).toBe('model');
    expect(prefs.order[1]).toBe('time');
    expect(prefs.order).toContain('protocol');
    expect(prefs.order).not.toContain('unknown');
    expect(prefs.widths.model).toBe(160);
    expect(prefs.visibility.model).toBe(false);
    expect(prefs.visibility.time).toBe(DEFAULT_COLUMN_VISIBILITY.time);
  });

  it('clamps widths to min/max', () => {
    const prefs = normalizeColumnPrefs({
      widths: { model: 10, time: 9999 },
    });
    expect(prefs.widths.model).toBeGreaterThanOrEqual(90);
    expect(prefs.widths.time).toBeLessThanOrEqual(480);
  });

  it('forces at least one visible column', () => {
    const visibility = Object.fromEntries(
      DEFAULT_COLUMN_ORDER.map((id) => [id, false]),
    ) as Record<string, boolean>;
    const prefs = normalizeColumnPrefs({ visibility });
    expect(prefs.visibility.time).toBe(true);
  });

  it('resolveVisibleColumns respects feature flags and visibility', () => {
    const prefs = createDefaultColumnPrefs();
    prefs.visibility.token = false;
    const visible = resolveVisibleColumns(prefs, {
      hasProjects: false,
      apiTokenAuthEnabled: true,
    });
    expect(visible).toContain('protocol');
    expect(visible).not.toContain('project');
    expect(visible).not.toContain('token');
  });

  it('persists column visibility, order, and widths in localStorage', () => {
    const prefs = createDefaultColumnPrefs();
    prefs.order = ['model', 'time', ...prefs.order.filter((id) => id !== 'model' && id !== 'time')];
    prefs.visibility.provider = false;
    prefs.widths.model = 220;

    writeColumnPrefs('scoped-columns', prefs);

    const stored = readColumnPrefs('scoped-columns');
    expect(stored.order.slice(0, 2)).toEqual(['model', 'time']);
    expect(stored.visibility.provider).toBe(false);
    expect(stored.widths.model).toBe(220);
  });

  it('migrates anonymous column prefs to the scoped user key', () => {
    const prefs = createDefaultColumnPrefs();
    prefs.order = ['cost', 'time', ...prefs.order.filter((id) => id !== 'cost' && id !== 'time')];
    prefs.visibility.cacheRead = false;
    prefs.widths.cost = 144;
    localStorage.setItem('maxx-requests-table-columns:anonymous', JSON.stringify(prefs));

    migrateColumnPrefs('maxx-requests-table-columns:tenant-1:user-1', [
      'maxx-requests-table-columns',
      'maxx-requests-table-columns:anonymous',
    ]);

    const migrated = readColumnPrefs('maxx-requests-table-columns:tenant-1:user-1');
    expect(migrated.order.slice(0, 2)).toEqual(['cost', 'time']);
    expect(migrated.visibility.cacheRead).toBe(false);
    expect(migrated.widths.cost).toBe(144);
    expect(localStorage.getItem('maxx-requests-table-columns:anonymous')).toBeNull();
  });

  it('does not overwrite existing scoped column prefs during migration', () => {
    const scoped = createDefaultColumnPrefs();
    scoped.visibility.model = false;
    const legacy = createDefaultColumnPrefs();
    legacy.visibility.provider = false;
    localStorage.setItem('scoped-columns', JSON.stringify(scoped));
    localStorage.setItem('legacy-columns', JSON.stringify(legacy));

    migrateColumnPrefs('scoped-columns', ['legacy-columns']);

    expect(readColumnPrefs('scoped-columns').visibility.model).toBe(false);
    expect(readColumnPrefs('scoped-columns').visibility.provider).toBe(true);
    expect(localStorage.getItem('legacy-columns')).not.toBeNull();
  });
});
