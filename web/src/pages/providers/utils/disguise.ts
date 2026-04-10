import type { DisguiseType, ProviderConfigCustomDisguise } from '@/lib/transport';

/**
 * Parse a comma- or newline-separated string of sensitive words into a
 * trimmed, deduplicated list (preserves first-occurrence order).
 */
export function parseSensitiveWords(value: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const raw of value.split(/[\n,]/)) {
    const trimmed = raw.trim();
    if (!trimmed || seen.has(trimmed)) continue;
    seen.add(trimmed);
    out.push(trimmed);
  }
  return out;
}

/**
 * Serialize the form's disguise sub-state into a `ProviderConfigCustomDisguise`
 * payload, or `undefined` when the state matches the legacy "claude-code with
 * all defaults" baseline. Returning `undefined` keeps the persisted config from
 * gaining a noisy default field on edit-and-save round trips for legacy providers.
 *
 * Shared between the create flow (`custom-config-step.tsx`) and the edit flow
 * (`provider-edit-flow.tsx`) so both produce identical payloads.
 */
export function buildDisguisePayload(
  disguiseType: DisguiseType | undefined,
  cloakMode: 'auto' | 'always' | 'never' | undefined,
  cloakStrictMode: boolean,
  cloakSensitiveWordsRaw: string,
): ProviderConfigCustomDisguise | undefined {
  const type: DisguiseType = disguiseType ?? 'claude-code';
  const mode = cloakMode ?? 'auto';
  const strict = cloakStrictMode;
  const words = parseSensitiveWords(cloakSensitiveWordsRaw || '');

  // Legacy default — claude-code with everything at defaults — is represented
  // as a missing `disguise` field in the persisted config.
  if (type === 'claude-code' && mode === 'auto' && !strict && words.length === 0) {
    return undefined;
  }

  if (type === 'claude-code') {
    return {
      type: 'claude-code',
      claudeCode: { mode, strictMode: strict, sensitiveWords: words },
    };
  }

  return { type };
}
