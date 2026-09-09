import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

function readSource(path: string) {
  return readFileSync(join(process.cwd(), path), 'utf8');
}

describe('provider and route row UI cleanup', () => {
  it('does not render the provider add-command copy affordance in provider rows', () => {
    const source = readSource('src/pages/providers/components/provider-row.tsx');

    expect(source).not.toContain('shareProviderAddCommand');
    expect(source).not.toContain('handleCopyProviderAddCommand');
    expect(source).not.toContain('buildProviderAddCommand');
    expect(source).not.toContain('navigator.clipboard');
  });

  it('keeps active route rows icon-only while preserving accessible labeling', () => {
    const source = readSource('src/pages/client-routes/components/provider-row.tsx');

    expect(source).toContain('aria-label={activeStreamingLabel}');
    expect(source).toContain('<Activity size={10} />');
    expect(source).not.toContain("{t('routes.providerActive')}");
  });

  it('keeps model mapping status switches icon-only without inline status text', () => {
    const source = readSource('src/pages/model-mappings/index.tsx');

    expect(source).toContain(
      "title={isMappingEnabled(rule) ? t('common.enabled') : t('common.disabled')}",
    );
    expect(source).not.toMatch(
      /<span[^>]*>\s*\{isMappingEnabled\(rule\) \? t\('common\.enabled'\) : t\('common\.disabled'\)\}\s*<\/span>/s,
    );
  });
});
