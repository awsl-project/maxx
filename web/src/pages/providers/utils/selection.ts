export function invertVisibleProviderSelection(
  selectedProviderIds: ReadonlySet<number>,
  visibleProviderIds: readonly number[],
): Set<number> {
  const next = new Set(selectedProviderIds);

  for (const providerId of visibleProviderIds) {
    if (next.has(providerId)) {
      next.delete(providerId);
    } else {
      next.add(providerId);
    }
  }

  return next;
}
