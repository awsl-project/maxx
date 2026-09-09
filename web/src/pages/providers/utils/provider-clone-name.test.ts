import { describe, expect, it } from 'vitest';
import { buildProviderCloneName } from './provider-clone-name';

const source = { id: 1, name: 'OpenAI 009' };

function name(settings?: Record<string, string>, providers = [{ id: 1, name: 'OpenAI 009' }]) {
  return buildProviderCloneName({
    provider: source,
    providers,
    settings,
    localizedSuffix: '（副本）',
  });
}

describe('buildProviderCloneName', () => {
  it('preserves the old suffix behavior by default', () => {
    expect(name()).toBe('OpenAI 009（副本）');
  });

  it('deduplicates suffix clones without mutating the source provider name', () => {
    expect(
      name(undefined, [
        source,
        { id: 2, name: 'OpenAI 009（副本）' },
        { id: 3, name: 'OpenAI 009（副本） 2' },
      ]),
    ).toBe('OpenAI 009（副本） 3');
  });

  it('does not clone to the exact source name when the source already has the localized suffix', () => {
    expect(
      buildProviderCloneName({
        provider: { id: 1, name: 'OpenAI（副本）' },
        providers: [{ id: 1, name: 'OpenAI（副本）' }],
        localizedSuffix: '（副本）',
      }),
    ).toBe('OpenAI（副本） 2');
  });

  it('does not clone to the exact source name when a template renders the source name', () => {
    expect(
      buildProviderCloneName({
        provider: source,
        providers: [source],
        settings: {
          provider_clone_name_strategy: 'template',
          provider_clone_name_template: '{{name}}',
        },
        localizedSuffix: '（副本）',
      }),
    ).toBe('OpenAI 009 2');
  });

  it('increments trailing numbers and preserves zero padding', () => {
    expect(name({ provider_clone_name_strategy: 'increment-number' })).toBe('OpenAI 010');
  });

  it('increments from the cloned provider name instead of the global max with the same prefix', () => {
    expect(
      buildProviderCloneName({
        provider: { id: 1, name: 'NVIDIA（3）' },
        providers: [
          { id: 1, name: 'NVIDIA（3）' },
          { id: 2, name: 'NVIDIA（100）' },
        ],
        settings: { provider_clone_name_strategy: 'increment-number' },
        localizedSuffix: '（副本）',
      }),
    ).toBe('NVIDIA（4）');
  });

  it('keeps incrementing the numeric token when the first numeric clone conflicts', () => {
    expect(
      name({ provider_clone_name_strategy: 'increment-number' }, [
        source,
        { id: 2, name: 'OpenAI 010' },
        { id: 3, name: 'OpenAI 011' },
      ]),
    ).toBe('OpenAI 012');
  });

  it('increments large numeric text without losing precision', () => {
    expect(
      buildProviderCloneName({
        provider: { id: 1, name: 'OpenAI 999999999999999999999999999999' },
        settings: { provider_clone_name_strategy: 'increment-number' },
        localizedSuffix: '（副本）',
      }),
    ).toBe('OpenAI 1000000000000000000000000000000');
  });

  it('uses the configured template and counter', () => {
    expect(
      name(
        {
          provider_clone_name_strategy: 'template',
          provider_clone_name_template: '{{base}}-clone-{{n}}',
        },
        [source, { id: 2, name: 'OpenAI 009-clone-2' }],
      ),
    ).toBe('OpenAI 009-clone-3');
  });
});
