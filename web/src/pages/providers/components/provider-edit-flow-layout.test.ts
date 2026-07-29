import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

const editFlowSource = readFileSync(
  join(process.cwd(), 'src/pages/providers/components/provider-edit-flow.tsx'),
  'utf8',
);

describe('ProviderEditFlow section layout', () => {
  it('keeps visibility and export in the same relative position as custom provider creation', () => {
    const clientConfig = editFlowSource.indexOf("t('provider.clientConfig')");
    const errorCooldown = editFlowSource.indexOf("t('provider.errorCooldownTitle')");
    const visibilityAndExport = editFlowSource.indexOf("t('provider.visibilityAndExport')");
    const supportModels = editFlowSource.indexOf('<ProviderSupportModels');

    expect(clientConfig).toBeGreaterThan(-1);
    expect(errorCooldown).toBeGreaterThan(clientConfig);
    expect(visibilityAndExport).toBeGreaterThan(errorCooldown);
    expect(supportModels).toBeGreaterThan(visibilityAndExport);
  });
});
