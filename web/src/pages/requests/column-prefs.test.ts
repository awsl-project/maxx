import { describe, expect, it } from 'vitest';
import {
  DEFAULT_COLUMN_ORDER,
  DEFAULT_COLUMN_VISIBILITY,
  DEFAULT_COLUMN_WIDTHS,
  createDefaultColumnPrefs,
  normalizeColumnPrefs,
  resolveVisibleColumns,
} from './column-prefs';

describe('column-prefs', () => {
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
});
