import { describe, expect, it } from 'vitest';
import { MODEL_MAPPING_PROVIDER_TYPE_OPTIONS } from './index';

describe('MODEL_MAPPING_PROVIDER_TYPE_OPTIONS', () => {
  it('exposes Claude and every configured provider type for global mapping rules', () => {
    const values = MODEL_MAPPING_PROVIDER_TYPE_OPTIONS.map((option) => option.value);

    expect(values).toContain('claude');
    expect(values).toContain('antigravity');
    expect(values).toContain('custom');
    expect(values).toContain('openrouter');
    expect(values).toContain('newapi');
    expect(new Set(values).size).toBe(values.length);
  });
});
