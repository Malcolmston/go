// copyText copies text to the clipboard on a best-effort basis: it tries the
// async Clipboard API, falls back to a hidden-textarea execCommand('copy'), and
// never rejects. The Clipboard API is gated on document focus/permission and
// can silently fail in some browsers, so callers should show "copied" feedback
// optimistically rather than await this.
export function copyText(text: string): void {
  try {
    const p = navigator.clipboard?.writeText(text);
    if (p && typeof p.catch === 'function') p.catch(() => fallbackCopy(text));
  } catch {
    fallbackCopy(text);
  }
}

function fallbackCopy(text: string): void {
  try {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    document.execCommand('copy');
    document.body.removeChild(ta);
  } catch {
    /* clipboard unavailable — nothing more we can do */
  }
}
