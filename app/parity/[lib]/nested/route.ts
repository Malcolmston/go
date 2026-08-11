// GET /parity/<harness>/nested — the nested package ports of ONE harness, as
// plain JSON.
//
// A repo like express carries 28 nested ports, each a full harness (its own
// cases, its own symbol inventory) against its OWN upstream package. Together
// they are ~94% of the harness JSON, and the /parity/<lib> page renders none of
// them until a reader opens a disclosure. Serialising them into that page's RSC
// payload cost over 1 MB per request for content nobody had asked for, so the
// page now ships only the collapsed-row summaries and the client fetches this
// route the first time a disclosure is opened.
//
// force-static + generateStaticParams means this is emitted as a file at build
// time, so the `output: 'export'` GitHub Pages build serves it as a static
// asset exactly like the Vercel build serves the prerendered route.

import { loadParityData, parityHarness, paritySlugs } from '../../load';

export const dynamic = 'force-static';

export function generateStaticParams() {
  return paritySlugs().map((lib) => ({ lib }));
}

// decodeURIComponent throws on a malformed escape; a bad URL must answer 404
// rather than throw out of the route.
function safeDecode(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ lib: string }> }
): Promise<Response> {
  const { lib } = await params;
  const found = parityHarness(safeDecode(lib));
  if (!found) {
    return Response.json({ error: 'unknown library' }, { status: 404 });
  }
  return Response.json({
    generatedAt: loadParityData().generatedAt,
    library: found.slug,
    nested: found.harness.nested,
  });
}
