import { describe, expect, it } from 'vitest';
import en from '../../../locales/en.json';
import zh from '../../../locales/zh.json';

const requiredKeys = [
  'button',
  'tooltip',
  'title',
  'description',
  'placeholder',
  'scopeHint',
  'updateFailed',
  'save',
] as const;

type CustomProviderKeyMessages = Record<(typeof requiredKeys)[number], string>;

function customProviderKeyMessages(locale: unknown): CustomProviderKeyMessages {
  return (locale as { routes: { customProviderKey: CustomProviderKeyMessages } }).routes
    .customProviderKey;
}

describe('custom provider key quick edit i18n', () => {
  it.each([
    ['en', customProviderKeyMessages(en)],
    ['zh', customProviderKeyMessages(zh)],
  ])('%s locale defines every quick-edit message', (_locale, messages) => {
    for (const key of requiredKeys) {
      expect(messages[key], key).toEqual(expect.any(String));
      expect(messages[key].trim(), key).not.toBe('');
    }
  });

  it('keeps the blank-key no-op hint localized', () => {
    expect(customProviderKeyMessages(en).description).toContain('Leave empty');
    expect(customProviderKeyMessages(en).placeholder).toContain('Leave empty');
    expect(customProviderKeyMessages(zh).description).toContain('留空');
    expect(customProviderKeyMessages(zh).placeholder).toContain('留空');
  });
});
