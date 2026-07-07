import { describe, expect, it } from 'vitest';

import { buildUserPanelChatCompletionsExample, buildUserPanelEndpointHints } from './user-panel-endpoints';

describe('user panel endpoints', () => {
  it('uses global proxy endpoints for managed user console keys', () => {
    const endpoints = buildUserPanelEndpointHints('https://maxx.example.com/');

    expect(endpoints).toEqual([
      { id: 'openai-codex', url: 'https://maxx.example.com/v1' },
      { id: 'claude', url: 'https://maxx.example.com' },
      {
        id: 'gemini',
        url: 'https://maxx.example.com/v1beta/models/{model}:generateContent',
      },
    ]);
  });

  it('does not append any project route segment by itself', () => {
    const endpoints = buildUserPanelEndpointHints('https://maxx.example.com');
    const example = buildUserPanelChatCompletionsExample({
      origin: 'https://maxx.example.com',
      tokenLabel: 'your key',
    });

    expect(endpoints.map((endpoint) => endpoint.url).join('\n')).not.toContain('/project/');
    expect(example).toContain('https://maxx.example.com/v1/chat/completions');
    expect(example).not.toContain('/project/');
  });
});
