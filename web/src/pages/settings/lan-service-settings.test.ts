import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const settingsPage = readFileSync(join(process.cwd(), 'src/pages/settings/index.tsx'), 'utf8');
const zh = readFileSync(join(process.cwd(), 'src/locales/zh.json'), 'utf8');
const en = readFileSync(join(process.cwd(), 'src/locales/en.json'), 'utf8');

describe('LAN service settings UI', () => {
  it('defaults the LAN service switch to enabled when the setting is missing', () => {
    expect(settingsPage).toContain("const LAN_ACCESS_ENABLED_SETTING_KEY = 'lan_access_enabled'");
    expect(settingsPage).toContain("settings?.[LAN_ACCESS_ENABLED_SETTING_KEY] !== 'false'");
  });

  it('persists the LAN service switch and surfaces restart affordance', () => {
    expect(settingsPage).toContain('handleLANAccessChange');
    expect(settingsPage).toContain("key: LAN_ACCESS_ENABLED_SETTING_KEY");
    expect(settingsPage).toContain("value: enabled ? 'true' : 'false'");
    expect(settingsPage).toContain("t('backendAddress.restartRequired')");
    expect(settingsPage).toContain('handleRestartServer');
  });

  it('ships localized copy for the user-facing LAN service controls', () => {
    for (const source of [zh, en]) {
      expect(source).toContain('lanService');
      expect(source).toContain('lanServiceDesc');
      expect(source).toContain('restartRequired');
      expect(source).toContain('restartNow');
    }
  });
});
