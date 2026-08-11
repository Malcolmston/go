// /parity/<harness> — one library's complete parity record: the pinned upstream
// oracle, the stages that specific run executed, the per-group split, the
// symbol-by-symbol inventory, every case, and each nested package port.
//
// A SERVER component so the heavy dataset stays on the build machine: only the
// selected library's slice crosses into the client bundle. generateStaticParams
// emits one page per harness for the static export.
import type { Metadata } from 'next';
import { notFound } from 'next/navigation';
import { ParityLibrary } from '../../../src/components/ParityLibrary';
import { nestedSummaries } from '../../../src/components/ParityNested';
import { notFoundMetadata } from '../../notFoundMetadata';
import { pageMetadata } from '../../seo';
import { libMeta, libRoute, loadParityData, parityHarness, paritySlugs } from '../load';

export function generateStaticParams() {
  return paritySlugs().map((lib) => ({ lib }));
}

// decodeURIComponent throws on a malformed escape; a bad URL must render the
// not-found body rather than throw out of the route.
function safeDecode(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ lib: string }>;
}): Promise<Metadata> {
  const { lib } = await params;
  const found = parityHarness(safeDecode(lib));
  // Unresolved slugs 404 in the page below; the metadata has to say noindex and
  // restate openGraph, or Next's shallow merge leaves the root layout's og:url
  // (the home page) on a dead end.
  if (!found) {
    return notFoundMetadata({
      title: 'Parity record not found',
      // Deliberately does not echo the requested slug: the description is the
      // one thing on this page a crawler or a social preview quotes verbatim,
      // and reflecting arbitrary URL text into it invites nonsense snippets.
      description:
        'No parity harness is published under that name. Browse the measured parity index for every library that has one.',
    });
  }
  const meta = libMeta(found.slug);
  const upstream = found.harness.upstream.raw || 'its upstream';
  return pageMetadata({
    title: `${meta.name} parity vs ${upstream}`,
    description: `Every case and every upstream symbol: how the ${meta.name} Go port compares to ${upstream}, measured by running both.`,
    path: `/parity/${found.slug}`,
  });
}

export default async function Page({ params }: { params: Promise<{ lib: string }> }) {
  const { lib } = await params;
  const found = parityHarness(safeDecode(lib));
  if (!found) notFound();
  const meta = libMeta(found.slug);
  // The nested harnesses are stripped out of the props: they are the bulk of the
  // dataset (express: 1.2 MB of the 1.3 MB harness) and ParityLibrary is a
  // client component, so anything passed here is serialised into this page's
  // RSC payload — for 28 disclosures that stay collapsed until a reader opens
  // one. Only the collapsed-row summaries travel with the page; the bodies come
  // from /parity/<slug>/nested on demand.
  const { nested, ...harness } = found.harness;
  return (
    <ParityLibrary
      slug={found.slug}
      name={meta.name}
      accent={meta.accent}
      repo={meta.repo}
      libRoute={libRoute(found.slug)}
      generatedAt={loadParityData().generatedAt}
      harness={{ ...harness, nested: {} }}
      nested={nestedSummaries(nested)}
    />
  );
}
