import { describe, expect, it } from 'vitest';
import { getProjectActivityState } from './project-activity';
import type { Project } from './transport';

const baseProject: Project = {
  id: 1,
  name: 'Demo',
  slug: 'demo',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  enabledCustomRoutes: [],
};

const now = Date.parse('2026-06-22T08:00:00Z');

describe('getProjectActivityState', () => {
  it('marks projects without request activity as never used cleanup candidates', () => {
    const state = getProjectActivityState(baseProject, 90, now);

    expect(state.status).toBe('never-used');
    expect(state.inactive).toBe(true);
    expect(state.unusedDays).toBeNull();
    expect(state.totalRequestCount).toBe(0);
  });

  it('uses last request time to decide inactive status', () => {
    const project: Project = {
      ...baseProject,
      lastRequestAt: '2026-03-01T08:00:00Z',
      lastSuccessfulRequestAt: '2026-02-28T08:00:00Z',
      requestCount30d: 0,
      successfulRequestCount30d: 0,
      totalRequestCount: 7,
    };

    const state = getProjectActivityState(project, 90, now);

    expect(state.status).toBe('inactive');
    expect(state.inactive).toBe(true);
    expect(state.unusedDays).toBe(113);
    expect(state.totalRequestCount).toBe(7);
  });

  it('keeps recently used projects active even when the last success is older', () => {
    const project: Project = {
      ...baseProject,
      lastRequestAt: '2026-06-21T08:00:00Z',
      lastSuccessfulRequestAt: '2026-02-01T08:00:00Z',
      requestCount30d: 3,
      successfulRequestCount30d: 0,
      totalRequestCount: 10,
    };

    const state = getProjectActivityState(project, 30, now);

    expect(state.status).toBe('active');
    expect(state.inactive).toBe(false);
    expect(state.unusedDays).toBe(1);
    expect(state.requestCount30d).toBe(3);
  });
});
