import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

const source = readFileSync(join(process.cwd(), 'src/pages/routing-strategies/index.tsx'), 'utf8');

describe('routing strategies page layout', () => {
  it('uses the shared page padding without an extra centered max-width wrapper', () => {
    expect(source).toContain('flex-1 overflow-auto p-4 md:p-6');
    expect(source).toContain('className="space-y-6"');
    expect(source).not.toContain('space-y-6 max-w-7xl mx-auto');
  });
});
