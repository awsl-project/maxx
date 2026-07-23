import type { ClientType, Route } from '@/lib/transport';
import { getTransport } from '@/lib/transport';
import type { RouteTtftProbeResult } from './ttft-sort';

interface ProbeInput {
  route: Route;
  clientType: ClientType;
  projectID: number;
}

export async function probeRoutesTtft(
  inputs: ProbeInput[],
  options: {
    concurrency?: number;
    signal?: AbortSignal;
    onResult?: (result: RouteTtftProbeResult) => void;
  } = {},
): Promise<RouteTtftProbeResult[]> {
  const response = await getTransport().probeRouteTTFT(
    {
      routeIDs: inputs.map((input) => input.route.id),
      clientType: inputs[0]?.clientType ?? 'claude',
      projectID: inputs[0]?.projectID ?? 0,
      concurrency: options.concurrency,
    },
    options.signal,
  );

  for (const result of response.results) {
    options.onResult?.(result);
  }

  return response.results.sort(
    (a, b) =>
      inputs.findIndex((item) => item.route.id === a.routeID) -
      inputs.findIndex((item) => item.route.id === b.routeID),
  );
}
