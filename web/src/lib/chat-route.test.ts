import { describe, expect, it } from 'vitest';
import {
  buildChatRequestBody,
  extractChatResponseText,
  extractChatStreamDelta,
  getChatEndpoint,
  getChatModelRoutesEndpoint,
  getExposedChatRouteTypes,
  normalizeChatModelRouteGroups,
  normalizeChatModelList,
  parseChatHashToken,
} from './chat-route';

describe('parseChatHashToken', () => {
  it('reads a plain hash token', () => {
    expect(parseChatHashToken('#maxx_secret')).toBe('maxx_secret');
  });

  it('reads a named token value', () => {
    expect(parseChatHashToken('#token=maxx_secret')).toBe('maxx_secret');
  });

  it('keeps plain tokens with equals signs and invalid escapes usable', () => {
    expect(parseChatHashToken('#maxx_part=a+b')).toBe('maxx_part=a+b');
    expect(parseChatHashToken('#maxx_bad%token')).toBe('maxx_bad%token');
  });
});

describe('model endpoints', () => {
  it('uses the user-panel exposed model route list endpoint', () => {
    expect(getChatModelRoutesEndpoint()).toBe('/api/chat/model-routes');
  });

  it('uses the selected Gemini model in the generation endpoint', () => {
    expect(getChatEndpoint('gemini', 'models/gemini-2.5-flash')).toBe(
      '/v1beta/models/gemini-2.5-flash:generateContent',
    );
  });

  it('uses the Gemini SSE streaming endpoint when streaming', () => {
    expect(getChatEndpoint('gemini', 'models/gemini-2.5-flash', { stream: true })).toBe(
      '/v1beta/models/gemini-2.5-flash:streamGenerateContent?alt=sse',
    );
  });
});

describe('normalizeChatModelRouteGroups', () => {
  it('keeps only exposed chat route groups with models and derives route types', () => {
    const groups = normalizeChatModelRouteGroups([
      { clientType: 'openai', models: ['gpt-5'] },
      { clientType: 'unknown', models: ['x'] },
      { clientType: 'claude', models: [] },
    ]);

    expect(groups).toEqual([{ clientType: 'openai', models: ['gpt-5'] }]);
    expect(getExposedChatRouteTypes(groups)).toEqual(['openai']);
  });
});

describe('normalizeChatModelList', () => {
  it('filters user-panel exposed model route groups by route type', () => {
    expect(
      normalizeChatModelList(
        [
          { clientType: 'openai', models: ['gpt-5', 'gpt-4o'] },
          { clientType: 'claude', models: ['claude-sonnet-4-5'] },
          { clientType: 'openai', models: ['gpt-5'] },
        ],
        'openai',
      ),
    ).toEqual(['gpt-4o', 'gpt-5']);
  });

  it('strips Gemini models/ prefixes from user-panel exposed names', () => {
    expect(
      normalizeChatModelList(
        [{ clientType: 'gemini', models: ['models/gemini-2.5-pro'] }],
        'gemini',
      ),
    ).toEqual(['gemini-2.5-pro']);
  });
});

describe('buildChatRequestBody', () => {
  it('builds OpenAI chat requests with the selected model', () => {
    expect(
      buildChatRequestBody('openai', 'gpt-4o', [{ role: 'user', content: 'hi' }]),
    ).toMatchObject({
      model: 'gpt-4o',
      messages: [{ role: 'user', content: 'hi' }],
      stream: false,
    });
  });

  it('builds Codex requests with conversation context', () => {
    expect(
      buildChatRequestBody('codex', 'gpt-5', [
        { role: 'user', content: 'hi' },
        { role: 'assistant', content: 'hello' },
      ]),
    ).toMatchObject({
      model: 'gpt-5',
      input: [
        { role: 'user', content: 'hi' },
        { role: 'assistant', content: 'hello' },
      ],
      stream: false,
    });
  });

  it('enables streaming on stream-capable request bodies', () => {
    expect(
      buildChatRequestBody('openai', 'gpt-4o', [{ role: 'user', content: 'hi' }], {
        stream: true,
      }),
    ).toMatchObject({ stream: true });
    expect(
      buildChatRequestBody('claude', 'claude-sonnet-4-5', [{ role: 'user', content: 'hi' }], {
        stream: true,
      }),
    ).toMatchObject({ stream: true });
    expect(
      buildChatRequestBody('codex', 'gpt-5', [{ role: 'user', content: 'hi' }], { stream: true }),
    ).toMatchObject({ stream: true });
  });

  it('builds Claude requests with the selected model', () => {
    expect(
      buildChatRequestBody('claude', 'claude-opus-4-5', [{ role: 'user', content: 'hi' }]),
    ).toMatchObject({
      model: 'claude-opus-4-5',
      messages: [{ role: 'user', content: 'hi' }],
    });
  });

  it('builds Gemini content requests', () => {
    expect(
      buildChatRequestBody('gemini', 'gemini-2.5-pro', [{ role: 'assistant', content: 'ok' }]),
    ).toEqual({
      contents: [{ role: 'model', parts: [{ text: 'ok' }] }],
    });
  });
});

describe('extractChatResponseText', () => {
  it('extracts OpenAI text', () => {
    expect(
      extractChatResponseText('openai', { choices: [{ message: { content: 'hello' } }] }),
    ).toBe('hello');
  });

  it('extracts Claude text', () => {
    expect(extractChatResponseText('claude', { content: [{ text: 'a' }, { text: 'b' }] })).toBe(
      'a\nb',
    );
  });
});

describe('extractChatStreamDelta', () => {
  it('extracts OpenAI, Claude, Gemini, and Codex stream deltas', () => {
    expect(extractChatStreamDelta('openai', { choices: [{ delta: { content: 'a' } }] })).toBe('a');
    expect(
      extractChatStreamDelta('claude', { type: 'content_block_delta', delta: { text: 'b' } }),
    ).toBe('b');
    expect(
      extractChatStreamDelta('gemini', {
        candidates: [{ content: { parts: [{ text: 'c' }, { text: 'd' }] } }],
      }),
    ).toBe('cd');
    expect(
      extractChatStreamDelta('codex', { type: 'response.output_text.delta', delta: 'e' }),
    ).toBe('e');
  });
});
