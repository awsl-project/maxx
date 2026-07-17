import { describe, expect, it } from 'vitest';

import { getVisibleProxyRouteClients, isProxyRouteVisible } from './proxy-route-exposure';

describe('proxy route exposure visibility', () => {
  it('uses production route defaults when settings are absent', () => {
    expect(isProxyRouteVisible(undefined, 'claude')).toBe(true);
    expect(isProxyRouteVisible(undefined, 'openai')).toBe(true);
    expect(isProxyRouteVisible(undefined, 'codex')).toBe(true);
    expect(isProxyRouteVisible(undefined, 'gemini')).toBe(false);
  });

  it('hides only routes explicitly set to false', () => {
    const settings = {
      proxy_route_claude_messages_enabled: 'false',
      proxy_route_openai_chat_enabled: 'true',
      proxy_route_responses_enabled: 'false',
      proxy_route_gemini_enabled: 'true',
    };

    expect(isProxyRouteVisible(settings, 'claude')).toBe(false);
    expect(isProxyRouteVisible(settings, 'openai')).toBe(true);
    expect(isProxyRouteVisible(settings, 'codex')).toBe(false);
    expect(isProxyRouteVisible(settings, 'gemini')).toBe(true);
  });

  it('filters client lists with the same visibility rules', () => {
    expect(
      getVisibleProxyRouteClients(
        {
          proxy_route_openai_chat_enabled: 'false',
          proxy_route_gemini_enabled: 'true',
        },
        ['claude', 'openai', 'codex', 'gemini'],
      ),
    ).toEqual(['claude', 'codex', 'gemini']);
  });
});
