import { describe, expect, it } from 'vitest';
import { isSupportedProxyURL, maskProxyURL, parseOutboundProxies } from './proxy-settings';

describe('proxy-settings', () => {
  it('parses only complete proxy definitions', () => {
    expect(
      parseOutboundProxies(
        JSON.stringify([
          { id: 'p1', name: 'Fast', url: 'http://user:pass@127.0.0.1:7890' },
          { id: '', name: 'bad', url: 'http://127.0.0.1:7890' },
        ]),
      ),
    ).toEqual([{ id: 'p1', name: 'Fast', url: 'http://user:pass@127.0.0.1:7890', disabled: false }]);
  });

  it('validates supported proxy schemes', () => {
    expect(isSupportedProxyURL('http://127.0.0.1:7890')).toBe(true);
    expect(isSupportedProxyURL('https://proxy.example:443')).toBe(true);
    expect(isSupportedProxyURL('socks5://127.0.0.1:7891')).toBe(true);
    expect(isSupportedProxyURL('socks5h://proxy.example:1080')).toBe(true);
    expect(isSupportedProxyURL('127.0.0.1:7890')).toBe(false);
  });

  it('masks proxy credentials in previews', () => {
    expect(maskProxyURL('http://user:pass@127.0.0.1:7890/')).toBe('http://***:***@127.0.0.1:7890/');
  });
});
