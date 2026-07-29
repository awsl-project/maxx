import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

const source = readFileSync(join(process.cwd(), 'src/pages/invite-codes/index.tsx'), 'utf8');
const zh = readFileSync(join(process.cwd(), 'src/locales/zh.json'), 'utf8');
const en = readFileSync(join(process.cwd(), 'src/locales/en.json'), 'utf8');

describe('invite code usage dialog copy', () => {
  it('shows the selected invite code in the title without the prefix placeholder label', () => {
    expect(source).toContain("t('inviteCodes.usagesTitleWithCode'");
    expect(source).not.toContain("`${t('inviteCodes.codePrefix')}: ${usageDialogCode.codePrefix}`");
    expect(zh).toContain('"usagesTitleWithCode": "邀请码 {{codePrefix}} 的使用记录"');
    expect(en).toContain('"usagesTitleWithCode": "Invite code {{codePrefix}} usages"');
  });
});
