import { describe, expect, it } from 'vitest';

import { PROVIDER_BULK_ACTIONS_STICKY_CLASS } from './index';

describe('provider bulk actions toolbar layout', () => {
  it('stays available inside the providers scroll container', () => {
    expect(PROVIDER_BULK_ACTIONS_STICKY_CLASS.split(' ')).toEqual(
      expect.arrayContaining(['sticky', 'top-0', 'z-20', 'flex', 'flex-wrap']),
    );
  });

  it('keeps an opaque readable surface while provider rows scroll under it', () => {
    expect(PROVIDER_BULK_ACTIONS_STICKY_CLASS).toContain('bg-background/95');
    expect(PROVIDER_BULK_ACTIONS_STICKY_CLASS).toContain('backdrop-blur');
    expect(PROVIDER_BULK_ACTIONS_STICKY_CLASS).toContain('shadow-sm');
  });
});
