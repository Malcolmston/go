// Minimal, dependency-free syntax highlighter + helpers, ported from the
// original static landing page so the React components render identical markup.

export function esc(s: string): string {
  return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

// hi highlights a Go/shell snippet into token <span>s. Input may already contain
// a few intentional <span class="tok-c">…</span> markers (used by the "integrate"
// samples); those are preserved.
export function hi(src: string): string {
  let s = esc(src);
  // un-escape any pre-baked tok-c spans passed through by callers
  s = s.replace(/&lt;span class=&quot;tok-c&quot;&gt;/g, '<span class="tok-c">')
       .replace(/&lt;\/span&gt;/g, '</span>');
  s = s.replace(/(&quot;[^&]*?&quot;|'[^']*?'|`[^`]*?`|"[^"]*?")/g, (m) => `<span class="tok-s">${m}</span>`);
  s = s.replace(/(\/\/[^\n]*)/g, (m) => `<span class="tok-c">${m}</span>`);
  s = s.replace(
    /\b(func|package|import|return|const|var|if|else|for|range|type|struct|interface|map|chan|go|defer|nil|any|require|new|string)\b/g,
    (m) => `<span class="tok-k">${m}</span>`
  );
  s = s.replace(/\b(\d+)\b/g, (m) => `<span class="tok-n">${m}</span>`);
  return s;
}

export const hx = (hex: string, a: string): string => hex + a;

// ghrepo derives the GitHub repo name from a library's repo URL,
// e.g. "https://github.com/malcolmston/socket.io" -> "socket.io".
export const ghrepo = (l: { repo: string }): string => l.repo.split('/').pop() ?? '';
