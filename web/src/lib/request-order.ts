type RequestOrderItem = {
  id: number;
  status: string;
};

export const ACTIVE_REQUEST_STATUSES = new Set(["PENDING", "IN_PROGRESS"]);

export function isActiveRequestStatus(status: string): boolean {
  return ACTIVE_REQUEST_STATUSES.has(status);
}

export function prioritizeActiveRequests<T extends RequestOrderItem>(requests: T[]): T[] {
  // WS 实时更新会原地改写 React Query 缓存；这里复用后端同一排序规则，
  // 让已结束请求在收到终态事件后立即下沉，而不是等到下一次整页 refetch。
  const active: T[] = [];
  const terminal: T[] = [];

  for (const request of requests) {
    if (isActiveRequestStatus(request.status)) {
      active.push(request);
      continue;
    }
    terminal.push(request);
  }

  active.sort((a, b) => b.id - a.id);
  terminal.sort((a, b) => b.id - a.id);

  return [...active, ...terminal];
}
