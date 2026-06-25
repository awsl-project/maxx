import { describe, expect, it } from 'vitest';
import { PROVIDER_TYPE_ORDER, createProviderTypeGroups, getKnownProviderTypeKey } from './types';

describe('provider type helpers', () => {
  it('keeps every configured provider type available for grouped provider UIs', () => {
    expect(PROVIDER_TYPE_ORDER).toEqual([
      'antigravity',
      'kiro',
      'codex',
      'claude',
      'bedrock',
      'custom',
    ]);
  });

  it('groups all known provider types and falls unknown providers back to custom', () => {
    const groups = createProviderTypeGroups([
      { name: 'Zulu Custom', type: 'custom' },
      { name: 'Codex Account', type: 'codex' },
      { name: 'Claude Account', type: 'claude' },
      { name: 'Bedrock Account', type: 'bedrock' },
      { name: 'Antigravity Account', type: 'antigravity' },
      { name: 'Kiro Account', type: 'kiro' },
      { name: 'Alpha Unknown', type: 'future-provider' },
    ]);

    expect(groups.antigravity.map((provider) => provider.name)).toEqual(['Antigravity Account']);
    expect(groups.kiro.map((provider) => provider.name)).toEqual(['Kiro Account']);
    expect(groups.codex.map((provider) => provider.name)).toEqual(['Codex Account']);
    expect(groups.claude.map((provider) => provider.name)).toEqual(['Claude Account']);
    expect(groups.bedrock.map((provider) => provider.name)).toEqual(['Bedrock Account']);
    expect(groups.custom.map((provider) => provider.name)).toEqual([
      'Alpha Unknown',
      'Zulu Custom',
    ]);
  });

  it('normalizes unknown provider types to the custom bucket', () => {
    expect(getKnownProviderTypeKey('codex')).toBe('codex');
    expect(getKnownProviderTypeKey('custom')).toBe('custom');
    expect(getKnownProviderTypeKey('future-provider')).toBe('custom');
  });
});
