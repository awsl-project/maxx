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
    const completeVMess = btoa(JSON.stringify({
      add: 'example.com',
      port: '443',
      id: '11111111-1111-1111-1111-111111111111',
    }));
    const incompleteVMess = btoa(JSON.stringify({ add: 'example.com' }));
    expect(isSupportedProxyURL('http://127.0.0.1:7890')).toBe(true);
    expect(isSupportedProxyURL('https://proxy.example:443')).toBe(true);
    expect(isSupportedProxyURL('socks5://127.0.0.1:7891')).toBe(true);
    expect(isSupportedProxyURL('socks5h://proxy.example:1080')).toBe(true);
    expect(isSupportedProxyURL(`vmess://${completeVMess}`)).toBe(true);
    expect(isSupportedProxyURL(`vmess://${incompleteVMess}`)).toBe(false);
    expect(isSupportedProxyURL('vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls')).toBe(true);
    expect(isSupportedProxyURL('trojan://secret@example.com:443')).toBe(true);
    expect(isSupportedProxyURL('ss://YWVzLTEyOC1nY206c2VjcmV0@example.com:8388')).toBe(true);
    expect(isSupportedProxyURL('127.0.0.1:7890')).toBe(false);
  });

  it('masks proxy credentials in previews', () => {
    expect(maskProxyURL('http://user:pass@127.0.0.1:7890/')).toBe('http://***:***@127.0.0.1:7890/');
    expect(maskProxyURL('vmess://eyJhZGQiOiJleGFtcGxlLmNvbSJ9')).toBe('vmess://***');
    expect(maskProxyURL('vless://secret@example.com:443?security=tls')).toBe('vless://***');
    expect(maskProxyURL('trojan://secret@example.com:443')).toBe('trojan://***');
    expect(maskProxyURL('ss://YWVzLTEyOC1nY206c2VjcmV0@example.com:8388')).toBe('ss://***');
  });
});
