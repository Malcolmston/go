import { describe, it, expect } from 'vitest';
import { esc, hi, hx, ghrepo } from 'go-ui';

// hi()'s output is injected with dangerouslySetInnerHTML by CodeBlock,
// CompareCard, SymbolCard and PackageView, so "no source text can become live
// markup" is a hard requirement, not a nicety.
describe('esc', () => {
  it('escapes the five HTML-significant characters', () => {
    expect(esc('<a href="x">&\'</a>')).toBe(
      '&lt;a href=&quot;x&quot;&gt;&amp;&#39;&lt;/a&gt;',
    );
  });

  it('escapes & before the other entities (no double-escaping artefacts)', () => {
    expect(esc('&lt;')).toBe('&amp;lt;');
  });

  it('coerces non-strings instead of throwing', () => {
    expect(esc(undefined as unknown as string)).toBe('');
    expect(esc(42 as unknown as string)).toBe('42');
  });
});

describe('hi', () => {
  const asElement = (html: string): HTMLElement => {
    const el = document.createElement('div');
    el.innerHTML = html;
    return el;
  };

  it('highlights keywords, strings, numbers and comments', () => {
    const html = hi('func main() { // hi\n  x := "a"\n  n := 12\n}');
    expect(html).toContain('<span class="tok-k">func</span>');
    expect(html).toContain('<span class="tok-c">// hi</span>');
    expect(html).toContain('<span class="tok-s">&quot;a&quot;</span>');
    expect(html).toContain('<span class="tok-n">12</span>');
  });

  it('escapes angle brackets so generics/HTML in source never become markup', () => {
    const html = hi('type S[T any] struct{ a <b> }');
    expect(html).toContain('&lt;b&gt;');
    expect(html).not.toContain('<b>');
  });

  it('cannot be made to emit a live element from source text', () => {
    const evil = '<img src=x onerror="alert(1)"><script>alert(2)</script>';
    const el = asElement(hi(evil));
    expect(el.querySelector('img')).toBeNull();
    expect(el.querySelector('script')).toBeNull();
    // Every emitted element is one of our own token spans.
    for (const child of Array.from(el.querySelectorAll('*'))) {
      expect(child.tagName).toBe('SPAN');
      expect(child.className).toMatch(/^tok-[kscn]$/);
    }
    expect(el.textContent).toBe(evil);
  });

  it('does not let a stray </span> in the source close our markup', () => {
    // Regression: the old implementation un-escaped every `&lt;/span&gt;`,
    // so source text containing `</span>` leaked raw markup into the DOM.
    const html = hi('x := "</span><b>oops</b>"');
    expect(html).not.toContain('<b>');
    expect(asElement(html).querySelector('b')).toBeNull();
  });

  it('preserves a pre-baked tok-c comment marker from the caller', () => {
    const html = hi('a\n<span class="tok-c">// note</span>\nb');
    const el = asElement(html);
    const comments = el.querySelectorAll('span.tok-c');
    expect(comments.length).toBe(1);
    expect(comments[0].textContent).toBe('// note');
    expect(el.textContent).toBe('a\n// note\nb');
  });

  it('keeps keywords inside strings and comments un-nested', () => {
    const el = asElement(hi('s := "func type"'));
    expect(el.querySelectorAll('span.tok-k').length).toBe(0);
    expect(el.querySelector('span.tok-s')?.textContent).toBe('"func type"');
  });

  it('handles an unterminated quote without swallowing the rest', () => {
    const el = asElement(hi('a := "oops\nfunc b()'));
    expect(el.querySelector('span.tok-k')?.textContent).toBe('func');
  });

  it('round-trips text content for arbitrary input', () => {
    const src = 'a & b < c > d "e" \'f\' `g` /* h */ 0xFF 1.5';
    expect(asElement(hi(src)).textContent).toBe(src);
  });

  it('returns an empty string for empty/undefined input', () => {
    expect(hi('')).toBe('');
    expect(hi(undefined as unknown as string)).toBe('');
  });
});

describe('hx / ghrepo', () => {
  it('hx concatenates a hex colour with an alpha suffix', () => {
    expect(hx('#00add8', '22')).toBe('#00add822');
  });

  it('ghrepo takes the last path segment', () => {
    expect(ghrepo({ repo: 'https://github.com/malcolmston/socket.io' })).toBe('socket.io');
  });

  it('ghrepo tolerates a trailing slash and a .git suffix', () => {
    expect(ghrepo({ repo: 'https://github.com/malcolmston/express/' })).toBe('express');
    expect(ghrepo({ repo: 'https://github.com/malcolmston/express.git' })).toBe('express');
  });

  it('ghrepo returns an empty string for an empty repo', () => {
    expect(ghrepo({ repo: '' })).toBe('');
  });
});
