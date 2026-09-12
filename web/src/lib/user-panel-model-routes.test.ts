import { describe, expect, it } from 'vitest';
import type { UserPanelAvailableModelRouteGroup } from '@/lib/transport';
import { visibleUserPanelModelRouteGroups } from './user-panel-model-routes';

function group(routeID: number, models: string[]): UserPanelAvailableModelRouteGroup {
  return {
    routeID,
    providerID: routeID + 100,
    providerName: `provider-${routeID}`,
    clientType: 'openai',
    projectID: 0,
    models,
  };
}

describe('visibleUserPanelModelRouteGroups', () => {
  it('shows at most four non-empty route groups', () => {
    const groups = visibleUserPanelModelRouteGroups([
      group(1, ['a']),
      group(2, []),
      group(3, ['b']),
      group(4, ['c']),
      group(5, ['d']),
      group(6, ['e']),
    ]);

    expect(groups.map((item) => item.routeID)).toEqual([1, 3, 4, 5]);
  });

  it('shows at most eight models per route and records hidden count', () => {
    const models = Array.from({ length: 10 }, (_, index) => `model-${index + 1}`);
    const [result] = visibleUserPanelModelRouteGroups([group(1, models)]);

    expect(result.visibleModels).toEqual(models.slice(0, 8));
    expect(result.hiddenModelCount).toBe(2);
  });
});
