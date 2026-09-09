import type { Provider } from '@/lib/transport';

export const PROVIDER_CLONE_NAME_STRATEGY_SETTING_KEY = 'provider_clone_name_strategy';
export const PROVIDER_CLONE_NAME_TEMPLATE_SETTING_KEY = 'provider_clone_name_template';

export type ProviderCloneNameStrategy = 'suffix' | 'increment-number' | 'template';

export interface BuildProviderCloneNameOptions {
  provider: Pick<Provider, 'id' | 'name'>;
  formName?: string;
  providers?: Array<Pick<Provider, 'id' | 'name'>>;
  settings?: Record<string, string>;
  localizedSuffix: string;
}

const DEFAULT_TEMPLATE = '{{name}}{{suffix}}';
const TRAILING_NUMBER_RE = /^(.*?)(\d+)(\D*)$/u;

function normalizeStrategy(value: string | undefined): ProviderCloneNameStrategy {
  if (value === 'increment-number' || value === 'template') return value;
  return 'suffix';
}

function normalizeBaseName(formName: string | undefined, fallback: string): string {
  return formName?.trim() || fallback;
}

function providerNameSet(providers: Array<Pick<Provider, 'id' | 'name'>> | undefined): Set<string> {
  return new Set((providers || []).map((candidate) => candidate.name.trim()).filter(Boolean));
}

function nextAvailableName(candidate: string, existingNames: Set<string>): string {
  const trimmed = candidate.trim();
  if (!existingNames.has(trimmed)) return trimmed;

  let index = 2;
  while (existingNames.has(`${trimmed} ${index}`)) {
    index += 1;
  }
  return `${trimmed} ${index}`;
}

function buildSuffixName(baseName: string, suffix: string): string {
  return baseName.endsWith(suffix) ? baseName : `${baseName}${suffix}`;
}

function incrementNumericText(digits: string): string {
  const chars = digits.split('');
  let carry = 1;

  for (let index = chars.length - 1; index >= 0; index -= 1) {
    const next = Number(chars[index]) + carry;
    if (next < 10) {
      chars[index] = String(next);
      carry = 0;
      break;
    }
    chars[index] = '0';
  }

  if (carry > 0) {
    chars.unshift('1');
  }

  return chars.join('');
}

function incrementTrailingNumber(
  baseName: string,
  fallbackSuffix: string,
  existingNames?: Set<string>,
): string {
  const match = baseName.match(TRAILING_NUMBER_RE);
  if (!match) return buildSuffixName(baseName, fallbackSuffix);

  const [, prefix, digits, suffix] = match;
  let nextDigits = incrementNumericText(digits);
  let candidate = `${prefix}${nextDigits}${suffix}`;

  while (existingNames?.has(candidate)) {
    nextDigits = incrementNumericText(nextDigits);
    candidate = `${prefix}${nextDigits}${suffix}`;
  }

  return candidate;
}

function renderTemplate(template: string, baseName: string, suffix: string, n: number): string {
  const fallbackBase = buildSuffixName(baseName, suffix);
  const rendered = template
    .replaceAll('{{name}}', baseName)
    .replaceAll('{{base}}', baseName.replace(new RegExp(`${escapeRegExp(suffix)}$`, 'u'), ''))
    .replaceAll('{{suffix}}', suffix)
    .replaceAll('{{n}}', String(n))
    .trim();
  return rendered || fallbackBase;
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function buildTemplateName(
  baseName: string,
  suffix: string,
  template: string | undefined,
  existingNames: Set<string>,
): string {
  const pattern = template?.trim() || DEFAULT_TEMPLATE;
  const hasCounter = pattern.includes('{{n}}');

  for (let n = 2; n < 10000; n += 1) {
    const candidate = renderTemplate(pattern, baseName, suffix, n);
    if (!existingNames.has(candidate)) return candidate;
    if (!hasCounter) break;
  }

  return nextAvailableName(renderTemplate(pattern, baseName, suffix, 2), existingNames);
}

export function buildProviderCloneName({
  provider,
  formName,
  providers,
  settings,
  localizedSuffix,
}: BuildProviderCloneNameOptions): string {
  const baseName = normalizeBaseName(formName, provider.name);
  const existingNames = providerNameSet(providers);
  const strategy = normalizeStrategy(settings?.[PROVIDER_CLONE_NAME_STRATEGY_SETTING_KEY]);

  if (strategy === 'increment-number') {
    return nextAvailableName(
      incrementTrailingNumber(baseName, localizedSuffix, existingNames),
      existingNames,
    );
  }

  if (strategy === 'template') {
    return buildTemplateName(
      baseName,
      localizedSuffix,
      settings?.[PROVIDER_CLONE_NAME_TEMPLATE_SETTING_KEY],
      existingNames,
    );
  }

  return nextAvailableName(buildSuffixName(baseName, localizedSuffix), existingNames);
}
