import type { ClientType, Provider, Route } from '@/lib/transport';
import type { RouteTtftProbeResult } from './ttft-sort';

const DEFAULT_MODELS: Record<ClientType, string> = {
  claude: 'claude-sonnet-4',
  openai: 'gpt-4o-mini',
  codex: 'gpt-5',
  gemini: 'gemini-2.5-flash',
};

const MAX_ERROR_LENGTH = 240;
const PROBE_TIMEOUT_MS = 30000;

interface ProbeInput {
  route: Route;
  provider: Provider;
  clientType: ClientType;
  projectID: number;
  signal?: AbortSignal;
}

interface ProbeRequestSpec {
  path: string;
  model: string;
  body: Record<string, unknown>;
}

function routeModelMapping(route: Route): Record<string, string> {
  return route.modelMapping ?? {};
}

function resolveProbeModel(route: Route, clientType: ClientType) {
  const requested = DEFAULT_MODELS[clientType];
  const mapping = routeModelMapping(route);
  return mapping[requested] || requested;
}

function buildProbeRequest(
  route: Route,
  provider: Provider,
  clientType: ClientType,
): ProbeRequestSpec {
  const model = resolveProbeModel(route, clientType);
  const providerPath = `/provider/${provider.id}`;

  switch (clientType) {
    case 'claude':
      return {
        path: `${providerPath}/v1/messages`,
        model,
        body: {
          model,
          max_tokens: 1,
          stream: true,
          messages: [{ role: 'user', content: 'Reply ok.' }],
        },
      };
    case 'openai':
      return {
        path: `${providerPath}/v1/chat/completions`,
        model,
        body: {
          model,
          max_tokens: 1,
          stream: true,
          messages: [{ role: 'user', content: 'Reply ok.' }],
        },
      };
    case 'codex':
      return {
        path: `${providerPath}/v1/responses`,
        model,
        body: {
          model,
          stream: true,
          max_output_tokens: 1,
          input: 'Reply ok.',
        },
      };
    case 'gemini':
      return {
        path: `${providerPath}/v1beta/models/${encodeURIComponent(model)}:generateContent`,
        model,
        body: {
          contents: [{ role: 'user', parts: [{ text: 'Reply ok.' }] }],
          generationConfig: { maxOutputTokens: 1 },
        },
      };
  }
}

async function readFirstChunk(response: Response, startedAt: number, signal?: AbortSignal) {
  if (!response.body) {
    return {
      metric: 'first_byte' as const,
      ttftMs: Math.max(0, Math.round(performance.now() - startedAt)),
    };
  }

  const reader = response.body.getReader();
  try {
    const first = await reader.read();
    if (signal?.aborted) throw new DOMException('aborted', 'AbortError');
    if (first.done) {
      return {
        metric: 'first_byte' as const,
        ttftMs: Math.max(0, Math.round(performance.now() - startedAt)),
      };
    }
    return {
      metric: 'ttft' as const,
      ttftMs: Math.max(0, Math.round(performance.now() - startedAt)),
    };
  } finally {
    reader.cancel().catch(() => undefined);
  }
}

function sanitizeError(error: unknown) {
  const message = error instanceof Error ? error.message : String(error ?? 'unknown error');
  return message.length > MAX_ERROR_LENGTH ? `${message.slice(0, MAX_ERROR_LENGTH)}…` : message;
}

export async function probeRouteTtft({
  route,
  provider,
  clientType,
  projectID,
  signal,
}: ProbeInput): Promise<RouteTtftProbeResult> {
  const spec = buildProbeRequest(route, provider, clientType);
  const startedAt = performance.now();
  const base = {
    routeID: route.id,
    providerID: provider.id,
    providerName: provider.name,
  };
  const timeoutController = new AbortController();
  const timeoutID = window.setTimeout(() => timeoutController.abort('timeout'), PROBE_TIMEOUT_MS);
  const relayAbort = () => timeoutController.abort('cancelled');
  signal?.addEventListener('abort', relayAbort, { once: true });

  try {
    const response = await fetch(spec.path, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'text/event-stream, application/json',
        'X-Maxx-Project-ID': String(projectID),
        'X-Maxx-TTFT-Probe': 'true',
      },
      body: JSON.stringify(spec.body),
      signal: timeoutController.signal,
    });

    if (!response.ok) {
      let error = response.statusText;
      try {
        const text = await response.clone().text();
        if (text.trim()) error = text.trim();
      } catch {
        // ignore body read errors; status is enough
      }
      return {
        ...base,
        ok: false,
        status: 'http_error',
        metric: 'none',
        httpStatus: response.status,
        durationMs: Math.max(0, Math.round(performance.now() - startedAt)),
        error: sanitizeError(error),
      };
    }

    const measured = await readFirstChunk(response, startedAt, timeoutController.signal);
    return {
      ...base,
      ok: true,
      status: 'success',
      metric: measured.metric,
      ttftMs: measured.ttftMs,
      httpStatus: response.status,
      durationMs: Math.max(0, Math.round(performance.now() - startedAt)),
    };
  } catch (error) {
    const aborted = error instanceof DOMException && error.name === 'AbortError';
    const timedOut = aborted && !signal?.aborted;
    return {
      ...base,
      ok: false,
      status: timedOut ? 'timeout' : aborted ? 'cancelled' : 'network_error',
      metric: 'none',
      durationMs: Math.max(0, Math.round(performance.now() - startedAt)),
      error: timedOut ? 'request timed out' : aborted ? 'request cancelled' : sanitizeError(error),
    };
  } finally {
    window.clearTimeout(timeoutID);
    signal?.removeEventListener('abort', relayAbort);
  }
}

export async function probeRoutesTtft(
  inputs: Array<Omit<ProbeInput, 'signal'>>,
  options: {
    concurrency?: number;
    signal?: AbortSignal;
    onResult?: (result: RouteTtftProbeResult) => void;
  } = {},
): Promise<RouteTtftProbeResult[]> {
  const concurrency = Math.max(1, Math.min(options.concurrency ?? 4, 4));
  const results: RouteTtftProbeResult[] = [];
  let nextIndex = 0;

  async function worker() {
    while (nextIndex < inputs.length && !options.signal?.aborted) {
      const input = inputs[nextIndex++];
      const result = await probeRouteTtft({ ...input, signal: options.signal });
      results.push(result);
      options.onResult?.(result);
    }
  }

  await Promise.all(Array.from({ length: Math.min(concurrency, inputs.length) }, () => worker()));
  return results.sort(
    (a, b) =>
      inputs.findIndex((item) => item.route.id === a.routeID) -
      inputs.findIndex((item) => item.route.id === b.routeID),
  );
}
