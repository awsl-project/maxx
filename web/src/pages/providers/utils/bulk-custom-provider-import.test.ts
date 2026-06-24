import { describe, expect, it } from 'vitest';
import {
  parseBulkCustomProviderCommands,
  tokenizeProviderCommand,
  toCreateProviderData,
} from './bulk-custom-provider-import';

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
