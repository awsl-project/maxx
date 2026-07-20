import { describe, expect, it } from 'vitest';
import { getRequestModelChain } from './model-chain';

describe('getRequestModelChain', () => {
  it('shows the request to mapped model chain without leaking response model labels', () => {
    const chain = getRequestModelChain({
      requestModel: 'claude-sonnet-4-5',
      mappedModel: 'openrouter/anthropic/claude-sonnet-4-5',
    });

    expect(chain).toEqual({
      requestModel: 'claude-sonnet-4-5',
      mappedModel: 'openrouter/anthropic/claude-sonnet-4-5',
      title:
        'Request model: claude-sonnet-4-5\nMapped model: openrouter/anthropic/claude-sonnet-4-5',
    });
    expect(chain.title).not.toContain('Response model');
    expect(chain.title).not.toContain('response:');
  });

  it('does not render a mapped leg when the mapped model equals the requested model', () => {
    expect(
      getRequestModelChain({ requestModel: 'gpt-4.1-mini', mappedModel: 'gpt-4.1-mini' }),
    ).toEqual({
      requestModel: 'gpt-4.1-mini',
      mappedModel: '',
      title: 'gpt-4.1-mini',
    });
  });
});
