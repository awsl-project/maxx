import type { UserPanelAvailableModelRouteGroup } from '@/lib/transport';

export type VisibleUserPanelModelRouteGroup = UserPanelAvailableModelRouteGroup & {
  visibleModels: string[];
  hiddenModelCount: number;
};

export function visibleUserPanelModelRouteGroups(
  groups: UserPanelAvailableModelRouteGroup[],
): VisibleUserPanelModelRouteGroup[] {
  return groups
    .filter((group) => group.models.length > 0)
    .slice(0, 4)
    .map((group) => ({
      ...group,
      visibleModels: group.models.slice(0, 8),
      hiddenModelCount: Math.max(group.models.length - 8, 0),
    }));
}
