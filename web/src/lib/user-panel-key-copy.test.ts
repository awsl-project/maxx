import { describe, expect, it, vi } from 'vitest';
import { copyUserPanelToken } from './user-panel-key-copy';

describe('copyUserPanelToken', () => {
  it('copies an already revealed token without revealing it again', async () => {
    const revealToken = vi.fn<() => Promise<string>>();
    const writeText = vi.fn<(_value: string) => Promise<void>>().mockResolvedValue(undefined);

    const token = await copyUserPanelToken({
      revealedToken: 'maxx_revealed',
      revealToken,
      writeText,
    });

    expect(token).toBe('maxx_revealed');
    expect(revealToken).not.toHaveBeenCalled();
    expect(writeText).toHaveBeenCalledWith('maxx_revealed');
  });

  it('reveals and copies the token when the visible value is masked', async () => {
    const revealToken = vi.fn<() => Promise<string>>().mockResolvedValue('maxx_secret');
    const writeText = vi.fn<(_value: string) => Promise<void>>().mockResolvedValue(undefined);

    const token = await copyUserPanelToken({ revealToken, writeText });

    expect(token).toBe('maxx_secret');
    expect(revealToken).toHaveBeenCalledTimes(1);
    expect(writeText).toHaveBeenCalledWith('maxx_secret');
  });

  it('does not copy a masked placeholder when reveal fails', async () => {
    const revealToken = vi.fn<() => Promise<string>>().mockRejectedValue(new Error('boom'));
    const writeText = vi.fn<(_value: string) => Promise<void>>().mockResolvedValue(undefined);

    await expect(copyUserPanelToken({ revealToken, writeText })).rejects.toThrow('boom');
    expect(writeText).not.toHaveBeenCalled();
  });
});
