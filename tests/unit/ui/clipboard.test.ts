import { describe, it, expect, vi, afterEach } from 'vitest';
import { copyText, copyTextAsync } from 'go-ui';

const setClipboard = (impl: unknown) => {
  Object.defineProperty(navigator, 'clipboard', { value: impl, configurable: true });
};

const setExecCommand = (impl: ((cmd: string) => boolean) | undefined) => {
  Object.defineProperty(document, 'execCommand', { value: impl, configurable: true });
};

afterEach(() => {
  setClipboard(undefined);
  setExecCommand(undefined);
});

describe('copyTextAsync', () => {
  it('uses the async Clipboard API when available', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    setClipboard({ writeText });
    await expect(copyTextAsync('hello')).resolves.toBe(true);
    expect(writeText).toHaveBeenCalledWith('hello');
  });

  it('falls back to execCommand when the Clipboard API rejects', async () => {
    const writeText = vi.fn().mockRejectedValue(new Error('NotAllowedError'));
    setClipboard({ writeText });
    const exec = vi.fn().mockReturnValue(true);
    setExecCommand(exec);

    await expect(copyTextAsync('hello')).resolves.toBe(true);
    expect(exec).toHaveBeenCalledWith('copy');
    // The temporary textarea must not be left behind.
    expect(document.querySelector('textarea')).toBeNull();
  });

  it('falls back when the Clipboard API is absent entirely', async () => {
    // Regression: the old implementation only wired the fallback to a rejected
    // promise, so a missing navigator.clipboard silently copied nothing.
    setClipboard(undefined);
    const exec = vi.fn().mockReturnValue(true);
    setExecCommand(exec);
    await expect(copyTextAsync('hello')).resolves.toBe(true);
    expect(exec).toHaveBeenCalledWith('copy');
  });

  it('reports failure when both paths fail, without throwing', async () => {
    setClipboard(undefined);
    setExecCommand(() => {
      throw new Error('nope');
    });
    await expect(copyTextAsync('hello')).resolves.toBe(false);
    expect(document.querySelector('textarea')).toBeNull();
  });

  it('restores focus to the previously focused element', async () => {
    setClipboard(undefined);
    setExecCommand(() => true);
    const btn = document.createElement('button');
    document.body.appendChild(btn);
    btn.focus();

    await copyTextAsync('hello');
    expect(document.activeElement).toBe(btn);
    btn.remove();
  });
});

describe('copyText', () => {
  it('never throws and never returns a rejected promise', async () => {
    setClipboard({ writeText: () => Promise.reject(new Error('denied')) });
    setExecCommand(undefined);
    expect(() => copyText('x')).not.toThrow();
    // Let the internal promise settle; an unhandled rejection would fail the run.
    await new Promise((r) => setTimeout(r, 0));
  });
});
