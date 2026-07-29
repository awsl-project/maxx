import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const page = readFileSync(join(process.cwd(), 'src/pages/api-tokens/index.tsx'), 'utf8');
const zh = readFileSync(join(process.cwd(), 'src/locales/zh.json'), 'utf8');
const en = readFileSync(join(process.cwd(), 'src/locales/en.json'), 'utf8');

describe('API token expired reactivation actions', () => {
  it('adds a bulk restore action for all expired tokens', () => {
    expect(page).toContain('handleReactivateExpiredTokens');
    expect(page).toContain('expiredTokens.map((token) =>');
    expect(page).toContain('updateToken.mutateAsync');
    expect(page).toContain('buildAPITokenReactivatePayload()');
    expect(page).toContain("t('apiTokens.reactivateExpired.button'");
  });

  it('disables row restore while bulk restore is running', () => {
    expect(page).toContain('isReactivatingExpired');
    expect(page).toContain('disabled={updateToken.isPending || isReactivatingExpired}');
  });

  it('ships localized bulk restore copy', () => {
    expect(zh).toContain('一键恢复有效 ({{count}})');
    expect(zh).toContain('上次已恢复 {{count}} 个已过期令牌。');
    expect(en).toContain('Reset validity ({{count}})');
    expect(en).toContain('Last reset restored {{count}} expired token(s).');
  });
});
