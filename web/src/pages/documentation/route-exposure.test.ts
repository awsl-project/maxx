import { describe, expect, it } from 'vitest';

import { getVisibleQuickstartClients, resolveVisibleQuickstartClient } from '../documentation';

describe('documentation quickstart route exposure', () => {
  it('hides disabled quickstart client tabs using public route settings', () => {
    expect(
      getVisibleQuickstartClients({
        proxy_route_claude_messages_enabled: 'false',
        proxy_route_openai_chat_enabled: 'true',
        proxy_route_responses_enabled: 'false',
        proxy_route_gemini_enabled: 'true',
      }),
    ).toEqual(['openai', 'gemini']);
  });

  it('falls back when the active quickstart tab is no longer visible', () => {
    expect(resolveVisibleQuickstartClient('claude', ['openai', 'codex'])).toBe('openai');
    expect(resolveVisibleQuickstartClient('codex', ['openai', 'codex'])).toBe('codex');
  });

  it('supports the all-disabled route exposure state', () => {
    expect(
      getVisibleQuickstartClients({
        proxy_route_claude_messages_enabled: 'false',
        proxy_route_openai_chat_enabled: 'false',
        proxy_route_responses_enabled: 'false',
        proxy_route_gemini_enabled: 'false',
      }),
    ).toEqual([]);
  });
});
