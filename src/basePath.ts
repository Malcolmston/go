// basePath returns the URL prefix the app is served under, which is '' on Vercel
// and in dev — the app is at the domain root.
//
// It exists because Next's own basePath only rewrites <Link> and asset URLs, not
// fetch(), so a manual fetch has to prefix it itself. The prefix was '/go' under
// the GitHub Pages export; that deployment has been removed, so nothing sets
// NEXT_PUBLIC_BASE_PATH today. The indirection is kept because withBase also
// normalises the leading slash for its 13 call sites, and because serving under a
// sub-path again would otherwise mean re-auditing every fetch.
export const BASE_PATH = process.env.NEXT_PUBLIC_BASE_PATH ?? '';
export function withBase(p: string): string {
  const b = BASE_PATH.replace(/\/$/, '');
  return `${b}/${p.replace(/^\//, '')}`;
}
