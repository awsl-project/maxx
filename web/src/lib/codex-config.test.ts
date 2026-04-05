import { describe, expect, it } from 'vitest';

import { __codexConfigTestUtils, buildCodexConfigBundle, buildProxyBaseUrl } from './codex-config';

describe('codex-config', () => {
  describe('ensureCodexBasePath', () => {
    it('appends /v1 for default proxy base url', () => {
      expect(__codexConfigTestUtils.ensureCodexBasePath('http://localhost:9880')).toBe(
        'http://localhost:9880/v1',
      );
    });

    it('appends /v1 for project-scoped proxy base url', () => {
      expect(
        __codexConfigTestUtils.ensureCodexBasePath('https://maxx.example.com/project/demo'),
      ).toBe('https://maxx.example.com/project/demo/v1');
    });

    it('keeps existing /v1 path stable', () => {
      expect(__codexConfigTestUtils.ensureCodexBasePath('https://maxx.example.com/v1')).toBe(
        'https://maxx.example.com/v1',
      );
    });
  });

  it('buildProxyBaseUrl preserves host fallback behavior', () => {
    expect(buildProxyBaseUrl({ address: 'localhost', port: 9880 })).toBe('http://localhost:9880');
  });

  it('buildCodexConfigBundle uses /v1 base url for responses wire api', () => {
    const bundle = buildCodexConfigBundle({
      token: 'maxx_test_token',
      baseUrl: 'http://localhost:9880/project/demo',
    });

    expect(bundle.baseUrl).toBe('http://localhost:9880/project/demo/v1');
    expect(bundle.configToml).toContain('base_url = "http://localhost:9880/project/demo/v1"');
    expect(bundle.configToml).toContain('wire_api = "responses"');
    expect(bundle.authJson).toContain('maxx_test_token');
  });
});
