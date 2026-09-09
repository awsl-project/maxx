import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

const source = readFileSync(
  join(process.cwd(), 'src/pages/client-routes/components/provider-row.tsx'),
  'utf8',
);

describe('client route provider active indicator', () => {
  it('keeps active request indicators visible even when cooldown UI is present', () => {
    expect(source).toContain('const hasActiveStreaming = enabled && streamingCount > 0;');
    expect(source).toContain('show={hasActiveStreaming}');
    expect(source).toContain('{hasActiveStreaming && (');
    expect(source).toContain('<StreamingBadge count={streamingCount} color={color} />');
    expect(source).not.toContain('enabled && streamingCount > 0 && !effectiveIsInCooldown && (');
  });

  it('uses active streaming as the row border and glow priority', () => {
    expect(source).toContain("hasActiveStreaming && 'ring-2 ring-offset-1 ring-offset-background'");
    expect(source).toContain('hasActiveStreaming\n            ? `${color}80`');
    expect(source).toContain('boxShadow: hasActiveStreaming ? `0 0 20px ${color}25` : undefined');
  });
});
