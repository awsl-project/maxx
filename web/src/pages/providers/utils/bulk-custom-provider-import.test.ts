import { describe, expect, it } from 'vitest';
import {
  buildCustomProviderShareCommand,
  parseBulkCustomProviderCommands,
  tokenizeProviderCommand,
  toCreateProviderData,
} from './bulk-custom-provider-import';
import type { ModelMapping, Provider } from '@/lib/transport';

describe('bulk custom provider import parser', () => {
  it('tokenizes quoted flag values', () => {
    expect(
      tokenizeProviderCommand(
        'provider add --name "Mimo Provider" --base-url "https://api.example.com/v1" --api-key sk-test',
      ),
    ).toEqual([
      'provider',
      'add',
      '--name',
      'Mimo Provider',
      '--base-url',
      'https://api.example.com/v1',
      '--api-key',
      'sk-test',
    ]);
  });

  it('parses support models, request mappings, and response mappings', () => {
    const result = parseBulkCustomProviderCommands(
      'provider add --name "Mimo" --base-url "https://api.example.com" --api-key sk-test --clients claude,openai --models claude-sonnet-4,gpt-5 --map claude-sonnet-4=upstream-sonnet,gpt-5=upstream-gpt --response-map upstream-sonnet=claude-sonnet-4',
    );

    expect(result.errors).toEqual([]);
    expect(result.commands).toHaveLength(1);
    expect(result.commands[0]).toMatchObject({
      name: 'Mimo',
      baseURL: 'https://api.example.com',
      apiKey: 'sk-test',
      clients: ['claude', 'openai'],
      supportModels: ['claude-sonnet-4', 'gpt-5'],
      modelMapping: {
        'claude-sonnet-4': 'upstream-sonnet',
        'gpt-5': 'upstream-gpt',
      },
      responseModelMapping: {
        'upstream-sonnet': 'claude-sonnet-4',
      },
    });
  });

  it('supports wildcard mapping to a fixed upstream model', () => {
    const result = parseBulkCustomProviderCommands(
      'provider add --name mimo --base-url https://api.example.com --api-key sk-test --clients claude --models "*" --map "* -> mimo-v2.5-pro"',
    );

    expect(result.errors).toEqual([]);
    expect(result.commands[0].supportModels).toEqual(['*']);
    expect(result.commands[0].modelMapping).toEqual({ '*': 'mimo-v2.5-pro' });
  });

  it('builds provider config with persisted mappings', () => {
    const result = parseBulkCustomProviderCommands(
      'provider add --name mimo --base-url https://api.example.com --api-key sk-test --clients claude --models claude-* --map "*=mimo-v2.5-pro" --response-map "mimo-v2.5-pro=claude-sonnet-4" --no-responses-passthrough',
    );

    const data = toCreateProviderData(result.commands[0]);

    expect(data).toMatchObject({
      type: 'custom',
      name: 'mimo',
      supportedClientTypes: ['claude'],
      supportModels: ['claude-*'],
      config: {
        custom: {
          baseURL: 'https://api.example.com',
          apiKey: 'sk-test',
          modelMapping: { '*': 'mimo-v2.5-pro' },
          responseModelMapping: { 'mimo-v2.5-pro': 'claude-sonnet-4' },
          responsesPassthrough: false,
        },
      },
    });
  });

  it('allows omitted response mapping and multiple mapping groups', () => {
    const result = parseBulkCustomProviderCommands(
      'provider add --name mimo --base-url https://api.example.com --api-key sk-test --clients claude --models claude-*,gpt-* --map "claude-*=mimo-claude,gpt-*=mimo-gpt" --map "*=mimo-v2.5-pro"',
    );

    expect(result.errors).toEqual([]);
    expect(result.commands[0]).toMatchObject({
      supportModels: ['claude-*', 'gpt-*'],
      modelMapping: {
        'claude-*': 'mimo-claude',
        'gpt-*': 'mimo-gpt',
        '*': 'mimo-v2.5-pro',
      },
      responseModelMapping: {},
    });

    const data = toCreateProviderData(result.commands[0]);
    expect(data.config?.custom?.responseModelMapping).toBeUndefined();
  });

  it('reports line-scoped errors and keeps valid commands', () => {
    const result = parseBulkCustomProviderCommands(`
provider add --name ok --base-url https://api.example.com --api-key sk-test --clients claude --map *=mimo-v2.5-pro
provider add --name bad --base-url https://api.example.com --api-key sk-test --clients unknown
`);

    expect(result.commands).toHaveLength(1);
    expect(result.errors).toEqual([
      { lineNumber: 3, message: 'Unsupported client "unknown"' },
      { lineNumber: 3, message: 'At least one client is required' },
    ]);
  });
});

describe('custom provider share command builder', () => {
  const baseProvider: Provider = {
    id: 1,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    type: 'custom',
    name: 'GLM Share Provider',
    config: {
      disableErrorCooldown: true,
      smartMappingRetryEnabled: true,
      smartMappingRetryLimit: 2,
      custom: {
        baseURL: 'https://api.example.com/v1',
        apiKey: 'sk-real-secret',
        modelMapping: { 'glm-5.2': 'z-ai/glm-5.2' },
        responseModelMapping: { 'z-ai/glm-5.2': 'glm-5.2' },
        responsesPassthrough: false,
      },
    },
    supportedClientTypes: ['openai', 'claude'],
    supportModels: ['glm-5.2'],
  };

  it('builds a bulk-import-compatible command without leaking the api key', () => {
    const command = buildCustomProviderShareCommand(baseProvider);

    expect(command).toBe(
      'provider add --name "GLM Share Provider" --base-url https://api.example.com/v1 --api-key "<YOUR_API_KEY>" --clients openai,claude --models glm-5.2 --map glm-5.2=z-ai/glm-5.2 --response-map z-ai/glm-5.2=glm-5.2 --disable-error-cooldown --smart-mapping-retry --smart-mapping-retry-limit 2 --no-responses-passthrough',
    );
    expect(command).not.toContain('sk-real-secret');
    expect(parseBulkCustomProviderCommands(command ?? '').errors).toEqual([]);
  });

  it('round-trips smart mapping retry flags into provider config', () => {
    const command =
      'provider add --name Smart --base-url https://api.example.com/v1 --api-key sk-test --clients openai --models gpt-5 --map gpt-5=upstream-a,gpt-5=upstream-b --disable-error-cooldown --smart-mapping-retry --smart-mapping-retry-limit 3';

    const parsed = parseBulkCustomProviderCommands(command);

    expect(parsed.errors).toEqual([]);
    expect(parsed.commands[0]).toMatchObject({
      disableErrorCooldown: true,
      smartMappingRetryEnabled: true,
      smartMappingRetryLimit: 3,
    });
    expect(toCreateProviderData(parsed.commands[0]).config).toMatchObject({
      disableErrorCooldown: true,
      smartMappingRetryEnabled: true,
      smartMappingRetryLimit: 3,
    });
  });

  it('rejects smart mapping retry without disabled error cooldown', () => {
    const parsed = parseBulkCustomProviderCommands(
      'provider add --name Smart --base-url https://api.example.com/v1 --api-key sk-test --clients openai --smart-mapping-retry',
    );

    expect(parsed.errors).toEqual([
      { lineNumber: 1, message: 'Smart mapping retry requires --disable-error-cooldown' },
    ]);
  });

  it('includes provider-scoped model mappings in the shared command', () => {
    const mappings: ModelMapping[] = [
      {
        id: 10,
        createdAt: '2026-01-01T00:00:00Z',
        updatedAt: '2026-01-01T00:00:00Z',
        scope: 'provider',
        clientType: '',
        providerType: 'custom',
        providerID: 1,
        projectID: 0,
        routeID: 0,
        apiTokenID: 0,
        pattern: 'gpt-5',
        target: 'upstream-gpt-5',
        priority: 0,
        isEnabled: true,
        isBuiltin: false,
      },
    ];

    expect(
      buildCustomProviderShareCommand(baseProvider, { providerModelMappings: mappings }),
    ).toContain('--map glm-5.2=z-ai/glm-5.2,gpt-5=upstream-gpt-5');
  });

  it('returns null for non-custom providers', () => {
    expect(buildCustomProviderShareCommand({ ...baseProvider, type: 'claude' })).toBeNull();
  });
});
