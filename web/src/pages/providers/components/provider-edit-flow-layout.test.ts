import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

const editFlowSource = readFileSync(
  join(process.cwd(), 'src/pages/providers/components/provider-edit-flow.tsx'),
  'utf8',
);
const createFlowSource = readFileSync(
  join(process.cwd(), 'src/pages/providers/components/custom-config-step.tsx'),
  'utf8',
);

function expectOrder(source: string, tokens: string[]) {
  let previous = -1;
  for (const token of tokens) {
    const index = source.indexOf(token);
    expect(index, token).toBeGreaterThan(-1);
    expect(index, token).toBeGreaterThan(previous);
    previous = index;
  }
}

describe('Provider create/edit section layout', () => {
  it('keeps edit provider sections in the expected order', () => {
    expectOrder(editFlowSource, [
      'id="provider-connection"',
      'id="provider-clients"',
      "t('provider.responsesPassthrough')",
      'id="provider-models"',
      '<ProviderSupportModels',
      'id="provider-policies"',
      "t('provider.quotaEnabled')",
      'id="provider-danger"',
      "t('provider.excludeFromExport')",
    ]);
  });

  it('keeps custom provider creation aligned with edit provider section order', () => {
    expectOrder(createFlowSource, [
      "t('provider.clientConfig')",
      "t('provider.responsesPassthrough')",
      '{/* Model Mapping Section */}',
      "t('provider.errorCooldownTitle')",
      "t('provider.visibilityAndExport')",
      "t('provider.excludeFromExport')",
    ]);
  });
});
