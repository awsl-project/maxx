import { describe, expect, it } from 'vitest';
import { normalizeGrokConfig } from './grok-token-import';

describe('normalizeGrokConfig', () => {
  it('accepts direct CPA xAI credential JSON', () => {
    expect(
      normalizeGrokConfig({
        type: 'xai',
        auth_kind: 'oauth',
        email: 'user@example.com',
        access_token: 'access-token',
        refresh_token: 'refresh-token',
        base_url: 'https://cli-chat-proxy.grok.com/v1',
      }),
    ).toMatchObject({
      type: 'xai',
      authKind: 'oauth',
      email: 'user@example.com',
      accessToken: 'access-token',
      refreshToken: 'refresh-token',
      baseURL: 'https://cli-chat-proxy.grok.com/v1',
    });
  });

  it('accepts CLIProxyAPI auth records with credentials in metadata', () => {
    expect(
      normalizeGrokConfig({
        provider: 'xai',
        disabled: true,
        attributes: { auth_kind: 'oauth' },
        metadata: {
          type: 'xai',
          email: 'record@example.com',
          access_token: 'record-access',
          refresh_token: 'record-refresh',
        },
      }),
    ).toMatchObject({
      type: 'xai',
      authKind: 'oauth',
      email: 'record@example.com',
      accessToken: 'record-access',
      refreshToken: 'record-refresh',
      disabled: true,
    });
  });

  it('accepts Maxx exported Grok providers with camelCase config', () => {
    expect(
      normalizeGrokConfig({
        type: 'grok',
        name: 'Grok export',
        config: {
          grok: {
            type: 'xai',
            authKind: 'oauth',
            email: 'maxx@example.com',
            accessToken: 'maxx-access',
            refreshToken: 'maxx-refresh',
          },
        },
      }),
    ).toMatchObject({
      type: 'xai',
      authKind: 'oauth',
      email: 'maxx@example.com',
      accessToken: 'maxx-access',
      refreshToken: 'maxx-refresh',
    });
  });
});
