import { describe, expect, it } from 'vitest';
import { buildUserPanelChatUrl } from './user-panel-chat-url';

describe('user-panel-chat-url', () => {
  it('builds a per-user chat URL with the token in the hash', () => {
    expect(buildUserPanelChatUrl('https://maxx.example/', 'maxx_user token')).toBe(
      'https://maxx.example/chat#maxx_user%20token',
    );
  });

  it('returns empty when origin or token is missing', () => {
    expect(buildUserPanelChatUrl('', 'maxx_abc')).toBe('');
    expect(buildUserPanelChatUrl('https://maxx.example', '')).toBe('');
  });
});
