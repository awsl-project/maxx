import { describe, expect, it } from 'vitest';
import {
  PROVIDER_TYPE_CONFIGS,
  PROVIDER_TYPE_ORDER,
  createProviderTypeGroups,
  getKnownProviderTypeKey,
  quickTemplates,
} from './types';

describe('provider type helpers', () => {
  it('keeps every configured provider type available for grouped provider UIs', () => {
    expect(PROVIDER_TYPE_ORDER).toEqual([
      'antigravity',
      'kiro',
      'codex',
      'claude',
      'bedrock',
      'openrouter',
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

  it('uses the versioned NVIDIA OpenAI-compatible API root in quick templates', () => {
    const nvidiaTemplate = quickTemplates.find((template) => template.id === 'nvidia');

    expect(nvidiaTemplate?.clientBaseURLs.openai).toBe('https://integrate.api.nvidia.com/v1');
  });

  it('uses the Zhipu OpenAI-compatible API root in quick templates', () => {
    const zhipuTemplate = quickTemplates.find((template) => template.id === 'zhipu');

    expect(zhipuTemplate?.supportedClients).toEqual(['openai']);
    expect(zhipuTemplate?.clientBaseURLs.openai).toBe('https://open.bigmodel.cn/api/paas/v4');
    expect(zhipuTemplate?.clientBaseURLs.claude).toBeUndefined();
  });

  it('adds DeepSeek OpenAI and Anthropic-compatible quick templates', () => {
    const deepseekOpenAI = quickTemplates.find((template) => template.id === 'deepseek-openai');
    const deepseekAnthropic = quickTemplates.find(
      (template) => template.id === 'deepseek-anthropic',
    );

    expect(deepseekOpenAI?.name).toBe('DeepSeek (OpenAI)');
    expect(deepseekOpenAI?.supportedClients).toEqual(['openai']);
    expect(deepseekOpenAI?.clientBaseURLs.openai).toBe('https://api.deepseek.com');

    expect(deepseekAnthropic?.name).toBe('DeepSeek (Anthropic)');
    expect(deepseekAnthropic?.supportedClients).toEqual(['claude']);
    expect(deepseekAnthropic?.clientBaseURLs.claude).toBe('https://api.deepseek.com/anthropic');
  });
});

it('hides black-box custom provider display info instead of leaking configured URLs', () => {
  const customConfig = PROVIDER_TYPE_CONFIGS.custom;

  expect(
    customConfig.getDisplayInfo({
      id: 1,
      createdAt: '',
      updatedAt: '',
      type: 'custom',
      name: 'black-box-provider',
      blackBox: true,
      config: {
        custom: {
          baseURL: 'https://hidden.example.com/v1',
          apiKey: 'secret-api-key',
        },
      },
      supportedClientTypes: ['openai'],
    }),
  ).toBe('Black box');
});
