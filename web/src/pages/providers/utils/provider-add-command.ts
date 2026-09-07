import type { Provider, ProviderConfigCustomDisguise } from '@/lib/transport';

function quoteShell(value: string): string {
  return `'${value.replace(/'/g, `'"'"'`)}'`;
}

function joinList(values: string[] | undefined): string {
  return (values ?? []).filter(Boolean).join(',');
}

function joinMapping(mapping: Record<string, string> | undefined): string {
  return Object.entries(mapping ?? {})
    .filter(([source, target]) => source && target)
    .map(([source, target]) => `${source}=${target}`)
    .join(',');
}

function pushValueFlag(parts: string[], flag: string, value: string | undefined) {
  const trimmed = value?.trim();
  if (!trimmed) return;
  parts.push(flag, quoteShell(trimmed));
}

function pushJSONFlag(parts: string[], flag: string, value: unknown) {
  if (value === undefined || value === null) return;
  parts.push(flag, quoteShell(JSON.stringify(value)));
}

function nonDefaultRecord<T>(
  value: Partial<Record<string, T>> | undefined,
): typeof value | undefined {
  return value && Object.keys(value).length > 0 ? value : undefined;
}

function providerDisguise(provider: Provider): ProviderConfigCustomDisguise | undefined {
  const custom = provider.config?.custom;
  if (custom?.disguise) return custom.disguise;

  const legacyCloak = (custom as { cloak?: ProviderConfigCustomDisguise['claudeCode'] } | undefined)
    ?.cloak;
  return legacyCloak ? { type: 'claude-code', claudeCode: legacyCloak } : undefined;
}

export function canBuildProviderAddCommand(provider: Provider): boolean {
  const custom = provider.config?.custom;
  if (provider.blackBox || provider.excludeFromExport) return false;
  if (provider.type !== 'custom' || !custom) return false;
  if (!custom.baseURL?.trim()) return false;
  if (custom.backend !== 'ollama' && !custom.apiKey?.trim()) return false;
  return true;
}

export function buildProviderAddCommand(provider: Provider): string | null {
  const custom = provider.config?.custom;
  if (!canBuildProviderAddCommand(provider) || !custom) return null;

  const parts = ['provider', 'add'];
  pushValueFlag(parts, '--name', provider.name);
  pushValueFlag(parts, '--base-url', custom.baseURL);
  pushValueFlag(parts, '--api-key', custom.apiKey);

  const clients = joinList(provider.supportedClientTypes);
  if (clients) parts.push('--clients', quoteShell(clients));

  const models = joinList(provider.supportModels);
  if (models) parts.push('--models', quoteShell(models));

  const exposedModels = joinList(provider.exposedModels);
  if (exposedModels) {
    parts.push('--exposed-models', quoteShell(exposedModels));
  } else if (provider.exposedModelsEnabled) {
    parts.push('--enable-exposed-models');
  }

  const modelMapping = joinMapping(custom.modelMapping);
  if (modelMapping) parts.push('--map', quoteShell(modelMapping));

  const responseModelMapping = joinMapping(custom.responseModelMapping);
  if (responseModelMapping) parts.push('--response-map', quoteShell(responseModelMapping));

  if (custom.backend === 'ollama') parts.push('--backend', 'ollama');
  pushValueFlag(parts, '--logo', provider.logo);
  pushJSONFlag(parts, '--client-base-url', nonDefaultRecord(custom.clientBaseURL));
  pushJSONFlag(parts, '--client-multiplier', nonDefaultRecord(custom.clientMultiplier));
  pushJSONFlag(parts, '--disguise', providerDisguise(provider));
  pushJSONFlag(parts, '--reasoning', provider.config?.reasoning);

  if (provider.config?.quotaEnabled) parts.push('--quota-enabled');
  if (provider.config?.disableErrorCooldown) parts.push('--disable-error-cooldown');
  if (provider.config?.consecutiveErrorFreezeEnabled) parts.push('--consecutive-error-freeze');
  if (provider.config?.consecutiveErrorFreezeThreshold) {
    parts.push(
      '--consecutive-error-freeze-threshold',
      String(provider.config.consecutiveErrorFreezeThreshold),
    );
  }
  if (provider.config?.smartMappingRetryEnabled) parts.push('--smart-mapping-retry');
  if (provider.config?.smartMappingRetryLimit && provider.config.smartMappingRetryLimit !== 1) {
    parts.push('--smart-mapping-retry-limit', String(provider.config.smartMappingRetryLimit));
  }
  if (provider.maxConcurrency && provider.maxConcurrency > 0) {
    parts.push('--max-concurrency', String(provider.maxConcurrency));
  }
  if (custom.responsesPassthrough === true) parts.push('--responses-passthrough');
  if (custom.responsesPassthrough === false) parts.push('--no-responses-passthrough');
  if (custom.responsesWebSocket === true) parts.push('--responses-websocket');

  return parts.join(' ');
}
