import { describe, it, expect, vi } from 'vitest';
import { resolveWorkBuddyConfig, safeWorkBuddyLoginURL } from './workbuddy-ai-form';
import { HttpTransport } from '@/lib/transport/http-transport';
import { getProviderCreatePath } from '../hooks/use-provider-navigation';
import { getKnownProviderTypeKey } from '../types';

describe('WorkBuddy international', () => {
  it('replaces enterprise metadata with the complete new personal authorization', () => {
    const old = {
      accessToken: 'old',
      refreshToken: 'old-refresh',
      enterpriseId: 'old-team',
      domain: 'old-domain',
      tokenType: 'old-type',
      refreshExpiresAt: 1,
    };
    const current = { accessToken: 'new', uid: 'personal' };
    expect(resolveWorkBuddyConfig(old, current, true)).toEqual(current);
    expect(resolveWorkBuddyConfig(old, {}, false)).toEqual(old);
    expect(resolveWorkBuddyConfig(old, { uid: 'edited' }, false)).toEqual({
      ...old,
      uid: 'edited',
    });
  });
  it('uses a first-class provider creation path', () => {
    expect(getKnownProviderTypeKey('workbuddy-ai')).toBe('workbuddy-ai');
    expect(getProviderCreatePath('workbuddy-ai')).toBe('/providers/create/workbuddy-ai');
  });
  it('rejects executable, insecure and credential-bearing login links', () => {
    expect(safeWorkBuddyLoginURL('javascript:alert(1)')).toBeNull();
    expect(safeWorkBuddyLoginURL('http://example.com')).toBeNull();
    expect(safeWorkBuddyLoginURL('https://dummy:dummy@example.com')).toBeNull();
    expect(safeWorkBuddyLoginURL('https://example.com/login?state=dummy')).toBe(
      'https://example.com/login?state=dummy',
    );
  });
  it('posts auth and model contracts only to admin transport', async () => {
    const transport = new HttpTransport({ baseURL: '/api' });
    const post = vi
      .fn()
      .mockResolvedValueOnce({
        data: { state: 'dummy', url: 'https://example.com', expiresAt: 123 },
      })
      .mockResolvedValueOnce({ data: { status: 'authorized', config: { accessToken: 'dummy' } } })
      .mockResolvedValueOnce({ data: { models: ['example-model'] } });
    Object.assign(transport, { adminClient: { post } });
    expect((await transport.startWorkBuddyAIAuth()).state).toBe('dummy');
    expect((await transport.pollWorkBuddyAIAuth('dummy')).config).toEqual({ accessToken: 'dummy' });
    expect(await transport.getWorkBuddyAIModels({ accessToken: 'dummy' })).toEqual({
      models: ['example-model'],
    });
    expect(post.mock.calls).toEqual([
      ['/workbuddy-ai/auth/start'],
      ['/workbuddy-ai/auth/poll', { state: 'dummy' }],
      ['/workbuddy-ai/models', { accessToken: 'dummy' }],
    ]);
  });
});
