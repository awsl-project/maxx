import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

const editFlowSource = readFileSync(
  join(process.cwd(), 'src/pages/providers/components/provider-edit-flow.tsx'),
  'utf8',
);

describe('ProviderEditFlow section layout', () => {
  it('keeps visibility and export in the same relative position as custom provider creation', () => {
    const connectionSection = editFlowSource.indexOf('id="provider-connection"');
    const clientSection = editFlowSource.indexOf('id="provider-clients"');
    const modelSection = editFlowSource.indexOf('id="provider-models"');
    const supportModels = editFlowSource.indexOf('<ProviderSupportModels');
    const policySection = editFlowSource.indexOf('id="provider-policies"');
    const quotaEnabled = editFlowSource.indexOf("t('provider.quotaEnabled')");
    const dangerSection = editFlowSource.indexOf('id="provider-danger"');

    expect(connectionSection).toBeGreaterThan(-1);
    expect(clientSection).toBeGreaterThan(connectionSection);
    expect(modelSection).toBeGreaterThan(clientSection);
    expect(supportModels).toBeGreaterThan(modelSection);
    expect(policySection).toBeGreaterThan(supportModels);
    expect(quotaEnabled).toBeGreaterThan(policySection);
    expect(dangerSection).toBeGreaterThan(policySection);
  });
});
