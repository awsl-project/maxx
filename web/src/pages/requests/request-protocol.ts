import type { ProxyRequest } from '@/lib/transport';

export type RequestProtocol = 'http' | 'sse' | 'websocket';

/**
 * Resolves the client→Maxx transport protocol for display.
 * Prefers the denormalized `protocol` field; falls back for historical rows.
 */
export function resolveRequestProtocol(
  request: Pick<ProxyRequest, 'protocol' | 'isStream' | 'statusCode'>,
): RequestProtocol {
  const raw = (request.protocol || '').trim().toLowerCase();
  if (raw === 'websocket' || raw === 'sse' || raw === 'http') {
    return raw;
  }
  // Completed Responses WebSocket turns record 101 Switching Protocols.
  if (request.statusCode === 101) {
    return 'websocket';
  }
  if (request.isStream) {
    return 'sse';
  }
  return 'http';
}

export function requestProtocolLabelKey(protocol: RequestProtocol): string {
  switch (protocol) {
    case 'websocket':
      return 'requests.protocolWebSocket';
    case 'sse':
      return 'requests.protocolSSE';
    default:
      return 'requests.protocolHTTP';
  }
}
