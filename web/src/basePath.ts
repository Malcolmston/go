// basePath returns the URL prefix the app is served under. On Vercel and in dev
// the app is at the domain root (''); the GitHub Pages static export serves it
// under /go. Manual fetch() calls must prefix this (Next's basePath only
// rewrites <Link>/asset URLs, not fetch). Mirrors the old Vite import.meta.env.BASE_URL.
export const BASE_PATH = process.env.NEXT_PUBLIC_BASE_PATH ?? '';
export function withBase(p: string): string {
  const b = BASE_PATH.replace(/\/$/, '');
  return `${b}/${p.replace(/^\//, '')}`;
}
