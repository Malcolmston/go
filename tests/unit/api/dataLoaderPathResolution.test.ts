// @vitest-environment node
//
// Source-level guard against a bug class that no behavioural unit test can catch.
//
// The loaders in api/_lib/ locate their generated JSON with
// `new URL(relativePath, import.meta.url)` + readFileSync. When the *specifier is
// an inline string literal*, webpack statically analyses the call and rewrites it
// into an asset reference — `__webpack_public_path__ + "static/media/<name>.<hash>.json"`
// — which is a URL string, not a readable filesystem path. In Next's route-handler
// layer that rewrite does happen, so readFileSync throws, and because every loader
// swallows read errors and degrades to an empty structure, the failure is SILENT:
// /api/health reported securityFindings: 0 while /security rendered all 23.
//
// Passing the specifier through a parameter (the readJson(relativePath) helper in
// both data.ts and security.ts) defeats the static analysis and leaves
// import.meta.url to resolve at runtime.
//
// Vitest does not bundle, so the loaders read correctly here either way — the only
// way to hold the line is to assert on the source text.
import { describe, it, expect } from 'vitest';
import fs from 'node:fs';
import { fileURLToPath } from 'node:url';

const libDir = fileURLToPath(new URL('../../../api/_lib/', import.meta.url));

// `new URL('<literal>', import.meta.url)` / "..." / `...` with no ${} inside.
const STATIC_URL_LITERAL =
  /new URL\(\s*(?:'[^']*'|"[^"]*"|`[^`$]*`)\s*,\s*import\.meta\.url\s*\)/g;

const loaderFiles = fs
  .readdirSync(libDir)
  .filter((f) => f.endsWith('.ts'))
  .sort();

// Comments discuss the broken shape on purpose (including in security.ts, which
// documents why the indirection exists), so only executable code is scanned.
function stripComments(src: string): string {
  return src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:])\/\/.*$/gm, '$1');
}

describe('api/_lib loaders resolve data paths dynamically', () => {
  it('has loader sources to check', () => {
    expect(loaderFiles).toContain('data.ts');
    expect(loaderFiles).toContain('security.ts');
  });

  it.each(loaderFiles)(
    '%s never inlines a literal specifier into new URL(..., import.meta.url)',
    (file) => {
      const src = stripComments(fs.readFileSync(libDir + file, 'utf8'));
      const hits = src.match(STATIC_URL_LITERAL) ?? [];
      expect(
        hits,
        `${file} resolves a data file with a statically-analysable literal, which webpack ` +
          `rewrites into an asset URL and breaks readFileSync at runtime. Pass the relative ` +
          `path through a helper parameter instead (see readJson() in data.ts).`
      ).toEqual([]);
    }
  );

  it('both JSON-backed loaders keep a readJson indirection helper', () => {
    for (const file of ['data.ts', 'security.ts']) {
      const src = fs.readFileSync(libDir + file, 'utf8');
      expect(src, `${file} should route reads through a helper`).toMatch(
        /function readJson\(\s*\w+\s*:\s*string\s*\)/
      );
      expect(src).toMatch(/new URL\(\s*\w+\s*,\s*import\.meta\.url\s*\)/);
    }
  });
});
