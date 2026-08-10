import { describe, expect, it } from 'vitest';

import {
  buildUserPanelChatCompletionsExample,
  buildUserPanelEndpointHints,
} from './user-panel-endpoints';

describe('user panel endpoints', () => {
  it('uses global proxy endpoints for managed user console keys', () => {
    const endpoints = buildUserPanelEndpointHints('https://maxx.example.com/');

    expect(endpoints).toEqual([
      { id: 'openai', url: 'https://maxx.example.com/v1' },
      { id: 'codex', url: 'https://maxx.example.com/v1' },
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
    });

    expect(endpoints.map((endpoint) => endpoint.url).join('\n')).not.toContain('/project/');
    expect(example).toContain('https://maxx.example.com/v1/chat/completions');
    expect(example).not.toContain('/project/');
    expect(example).not.toContain('Authorization: Bearer');
  });

  it('filters endpoint hints by public route exposure settings', () => {
    expect(
      buildUserPanelEndpointHints('https://maxx.example.com', {
        proxy_route_claude_messages_enabled: 'false',
        proxy_route_openai_chat_enabled: 'false',
        proxy_route_responses_enabled: 'true',
        proxy_route_gemini_enabled: 'true',
      }),
    ).toEqual([
      { id: 'codex', url: 'https://maxx.example.com/v1' },
      {
        id: 'gemini',
        url: 'https://maxx.example.com/v1beta/models/{model}:generateContent',
      },
    ]);
  });

  it('hides OpenAI and Codex endpoints independently while leaving Gemini default-visible', () => {
    expect(
      buildUserPanelEndpointHints('https://maxx.example.com', {
        proxy_route_openai_chat_enabled: 'false',
        proxy_route_responses_enabled: 'false',
      }).map((endpoint) => endpoint.id),
    ).toEqual(['claude', 'gemini']);
  });
});
