// Test-only stub for `next/navigation`.
//
// Explore (and any other client view) calls useRouter()/usePathname() at render
// time. The real hooks read the App Router context, which does not exist under
// vitest/jsdom and throws ("invariant expected app router to be mounted"). This
// stub returns inert, no-op equivalents so the components render; the unit tests
// exercise the components' own behaviour, not the router itself.
//
// Wired in via `resolve.alias` in vitest.config.ts; it never ships in a build.

export interface StubRouter {
  push: (href: string) => void;
  replace: (href: string) => void;
  back: () => void;
  forward: () => void;
  refresh: () => void;
  prefetch: (href: string) => void;
}

// A single stable router instance so identity is preserved across renders.
const router: StubRouter = {
  push: () => {},
  replace: () => {},
  back: () => {},
  forward: () => {},
  refresh: () => {},
  prefetch: () => {},
};

export function useRouter(): StubRouter {
  return router;
}

export function usePathname(): string {
  return '/';
}

export function useParams<T extends Record<string, string | string[]>>(): T {
  return {} as T;
}

export function useSearchParams(): URLSearchParams {
  return new URLSearchParams();
}

export function useSelectedLayoutSegment(): string | null {
  return null;
}

export function useSelectedLayoutSegments(): string[] {
  return [];
}

export function redirect(_url: string): never {
  throw new Error(`stub redirect: ${_url}`);
}

export function permanentRedirect(_url: string): never {
  throw new Error(`stub permanentRedirect: ${_url}`);
}

export function notFound(): never {
  throw new Error('stub notFound');
}
