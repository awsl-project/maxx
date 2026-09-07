import { describe, expect, it } from 'vitest';
import type { Provider } from '@/lib/transport';
import { buildProviderAddCommand, canBuildProviderAddCommand } from './provider-add-command';
import {
  parseBulkCustomProviderCommands,
  toCreateProviderData,
} from './bulk-custom-provider-import';

const baseProvider: Provider = {
  id: 1,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  type: 'custom',
  name: "Mimo's Provider",
  logo: 'https://example.com/logo.png',
  supportedClientTypes: ['claude', 'openai', 'codex'],
  supportModels: ['claude-*', 'gpt-5'],
  exposedModelsEnabled: true,
  exposedModels: ['claude-sonnet-4', 'gpt-5'],
  maxConcurrency: 7,
  excludeFromExport: false,
  blackBox: false,
  config: {
    quotaEnabled: true,
    disableErrorCooldown: true,
    consecutiveErrorFreezeEnabled: true,
    consecutiveErrorFreezeThreshold: 5,
    smartMappingRetryEnabled: true,
    smartMappingRetryLimit: 3,
    reasoning: { defaultEffort: 'low', maxEffort: 'high' },
    custom: {
      baseURL: 'https://api.example.com/v1',
      apiKey: "sk-test'quoted",
      clientBaseURL: {
        openai: 'https://openai.example.com/v1',
        claude: 'https://claude.example.com',
      },
      clientMultiplier: {
        openai: 12000,
      },
      disguise: {
        type: 'claude-code',
        claudeCode: {
          mode: 'always',
          strictMode: true,
          sensitiveWords: ['secret'],
        },
      },
      modelMapping: {
        'claude-*': 'upstream-sonnet',
        'gpt-5': 'upstream-gpt',
      },
      responseModelMapping: {
        'upstream-sonnet': 'claude-sonnet-4',
      },
      responsesPassthrough: false,
      responsesWebSocket: true,
    },
  },
};

describe('provider-add-command', () => {
  it('builds a provider add command that round-trips through the bulk parser', () => {
    const command = buildProviderAddCommand(baseProvider);

    expect(command).toContain("--name 'Mimo'\"'\"'s Provider'");
    expect(command).toContain("--api-key 'sk-test'\"'\"'quoted'");
    expect(command).toContain('--disable-error-cooldown');
    expect(command).toContain('--quota-enabled');
    expect(command).toContain('--disable-error-cooldown');
    expect(command).toContain('--consecutive-error-freeze');
    expect(command).toContain('--consecutive-error-freeze-threshold 5');
    expect(command).toContain('--smart-mapping-retry');
    expect(command).toContain('--smart-mapping-retry-limit 3');
    expect(command).toContain('--max-concurrency 7');
    expect(command).toContain("--exposed-models 'claude-sonnet-4,gpt-5'");
    expect(command).toContain('--client-base-url');
    expect(command).toContain('--client-multiplier');
    expect(command).toContain('--disguise');
    expect(command).toContain('--reasoning');
    expect(command).toContain('--no-responses-passthrough');
    expect(command).toContain('--responses-websocket');

    const parsed = parseBulkCustomProviderCommands(command ?? '');
    expect(parsed.errors).toEqual([]);
    expect(parsed.commands).toHaveLength(1);

    const data = toCreateProviderData(parsed.commands[0]);
    expect(data.name).toBe(baseProvider.name);
    expect(data.config?.custom?.baseURL).toBe(baseProvider.config?.custom?.baseURL);
    expect(data.config?.custom?.apiKey).toBe(baseProvider.config?.custom?.apiKey);
    expect(data.supportedClientTypes).toEqual(baseProvider.supportedClientTypes);
    expect(data.supportModels).toEqual(baseProvider.supportModels);
    expect(data.exposedModelsEnabled).toBe(true);
    expect(data.exposedModels).toEqual(baseProvider.exposedModels);
    expect(data.config?.custom?.modelMapping).toEqual(baseProvider.config?.custom?.modelMapping);
    expect(data.config?.custom?.responseModelMapping).toEqual(
      baseProvider.config?.custom?.responseModelMapping,
    );
    expect(data.config?.quotaEnabled).toBe(true);
    expect(data.config?.disableErrorCooldown).toBe(true);
    expect(data.config?.consecutiveErrorFreezeEnabled).toBe(true);
    expect(data.config?.consecutiveErrorFreezeThreshold).toBe(5);
    expect(data.config?.smartMappingRetryEnabled).toBe(true);
    expect(data.config?.smartMappingRetryLimit).toBe(3);
    expect(data.config?.reasoning).toEqual(baseProvider.config?.reasoning);
    expect(data.maxConcurrency).toBe(7);
    expect(data.config?.custom?.clientBaseURL).toEqual(baseProvider.config?.custom?.clientBaseURL);
    expect(data.config?.custom?.clientMultiplier).toEqual(
      baseProvider.config?.custom?.clientMultiplier,
    );
    expect(data.config?.custom?.disguise).toEqual(baseProvider.config?.custom?.disguise);
    expect(data.config?.custom?.responsesPassthrough).toBe(false);
    expect(data.config?.custom?.responsesWebSocket).toBe(true);
  });

  it('does not export stale exposed models when the allowlist is disabled', () => {
    const provider: Provider = {
      ...baseProvider,
      exposedModelsEnabled: false,
      exposedModels: ['claude-sonnet-4'],
    };

    const command = buildProviderAddCommand(provider);
    expect(command).not.toContain('--exposed-models');
    expect(command).not.toContain('--enable-exposed-models');

    const parsed = parseBulkCustomProviderCommands(command ?? '');
    expect(parsed.errors).toEqual([]);
    const data = toCreateProviderData(parsed.commands[0]);
    expect(data.exposedModelsEnabled).toBe(false);
    expect(data.exposedModels).toEqual([]);
  });

  it('preserves an enabled empty exposed model allowlist', () => {
    const provider: Provider = {
      ...baseProvider,
      exposedModelsEnabled: true,
      exposedModels: [],
    };

    const command = buildProviderAddCommand(provider);
    expect(command).toContain('--enable-exposed-models');
    expect(command).not.toContain('--exposed-models');

    const parsed = parseBulkCustomProviderCommands(command ?? '');
    expect(parsed.errors).toEqual([]);
    expect(toCreateProviderData(parsed.commands[0]).exposedModelsEnabled).toBe(true);
    expect(toCreateProviderData(parsed.commands[0]).exposedModels).toEqual([]);
  });

  it('supports ollama providers without api keys', () => {
    const provider: Provider = {
      ...baseProvider,
      config: {
        custom: {
          baseURL: 'http://localhost:11434/v1',
          apiKey: '',
          backend: 'ollama',
        },
      },
      supportedClientTypes: ['openai'],
      supportModels: ['llama3.1'],
    };

    expect(canBuildProviderAddCommand(provider)).toBe(true);
    const command = buildProviderAddCommand(provider);
    expect(command).toContain('--backend ollama');

    const parsed = parseBulkCustomProviderCommands(command ?? '');
    expect(parsed.errors).toEqual([]);
  });

  it('refuses providers that cannot be safely shared as provider add commands', () => {
    expect(canBuildProviderAddCommand({ ...baseProvider, type: 'claude' })).toBe(false);
    expect(canBuildProviderAddCommand({ ...baseProvider, blackBox: true })).toBe(false);
    expect(canBuildProviderAddCommand({ ...baseProvider, excludeFromExport: true })).toBe(false);
    expect(buildProviderAddCommand({ ...baseProvider, blackBox: true })).toBeNull();
    expect(buildProviderAddCommand({ ...baseProvider, excludeFromExport: true })).toBeNull();
    expect(
      canBuildProviderAddCommand({
        ...baseProvider,
        config: { custom: { baseURL: '', apiKey: 'sk-test' } },
      }),
    ).toBe(false);
    expect(
      canBuildProviderAddCommand({
        ...baseProvider,
        config: { custom: { baseURL: 'https://api.example.com', apiKey: '' } },
      }),
    ).toBe(false);
  });
});
