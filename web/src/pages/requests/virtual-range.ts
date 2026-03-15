export interface VirtualRange {
  startIndex: number;
  endIndex: number;
  topSpacerHeight: number;
  bottomSpacerHeight: number;
}

export const REQUEST_LIST_OVERSCAN = 8;

export function calculateVirtualRange(
  itemCount: number,
  scrollTop: number,
  viewportHeight: number,
  itemHeight: number,
  overscan = REQUEST_LIST_OVERSCAN,
): VirtualRange {
  if (itemCount <= 0 || viewportHeight <= 0 || itemHeight <= 0) {
    return {
      startIndex: 0,
      endIndex: itemCount,
      topSpacerHeight: 0,
      bottomSpacerHeight: 0,
    };
  }

  // 只渲染可视区域附近的行；overscan 用来避免滚动时出现白屏。
  const visibleCount = Math.ceil(viewportHeight / itemHeight);
  const rawStartIndex = Math.floor(scrollTop / itemHeight);
  const startIndex = Math.max(0, rawStartIndex - overscan);
  const endIndex = Math.min(itemCount, rawStartIndex + visibleCount + overscan);

  return {
    startIndex,
    endIndex,
    topSpacerHeight: startIndex * itemHeight,
    bottomSpacerHeight: Math.max(0, (itemCount - endIndex) * itemHeight),
  };
}
