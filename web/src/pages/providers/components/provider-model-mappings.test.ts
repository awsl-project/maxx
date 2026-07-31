import { describe, expect, it } from 'vitest';
import { buildProviderRuntimeModelOptions } from './provider-model-mappings';

describe('buildProviderRuntimeModelOptions', () => {
  it('puts provider interface models before configured support models and mapping targets', () => {
    const options = buildProviderRuntimeModelOptions(
      ['gpt-4o', 'gpt-4.1'],
      ['configured-only', 'gpt-4o'],
      ['mapped-only', 'gpt-4.1'],
      'Current provider models',
    );

    expect(options.map((option) => option.id)).toEqual([
      'gpt-4o',
      'gpt-4.1',
      'configured-only',
      'mapped-only',
    ]);
  });
});
