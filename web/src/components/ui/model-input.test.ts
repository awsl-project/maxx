import { describe, expect, it } from 'vitest';
import { buildModelOptions } from './model-input';

describe('buildModelOptions', () => {
  it('prepends runtime provider models before built-in presets', () => {
    const options = buildModelOptions([
      { id: 'provider-live-alpha', name: 'provider-live-alpha', provider: 'Current provider models' },
      { id: 'provider-live-beta', name: 'provider-live-beta', provider: 'Current provider models' },
    ]);

    expect(options.slice(0, 2).map((model) => model.id)).toEqual([
      'provider-live-alpha',
      'provider-live-beta',
    ]);
    expect(options[2].id).toBe('*claude*');
  });

  it('does not duplicate a preset when a runtime provider model has the same id', () => {
    const options = buildModelOptions([
      { id: 'gpt-4o', name: 'Runtime GPT-4o', provider: 'Current provider models' },
    ]);

    expect(options[0]).toMatchObject({ id: 'gpt-4o', provider: 'Current provider models' });
    expect(options.filter((model) => model.id === 'gpt-4o')).toHaveLength(1);
  });
});
