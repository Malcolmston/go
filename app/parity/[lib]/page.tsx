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
  if (!found) return { title: 'Parity record not found' };
  const meta = libMeta(found.slug);
  const upstream = found.harness.upstream.raw || 'its upstream';
  return {
    title: `${meta.name} parity vs ${upstream}`,
    description: `Every case and every upstream symbol: how the ${meta.name} Go port compares to ${upstream}, measured by running both.`,
  };
}

export default async function Page({ params }: { params: Promise<{ lib: string }> }) {
  const { lib } = await params;
  const found = parityHarness(safeDecode(lib));
  if (!found) notFound();
  const meta = libMeta(found.slug);
  return (
    <ParityLibrary
      slug={found.slug}
      name={meta.name}
      accent={meta.accent}
      repo={meta.repo}
      libRoute={libRoute(found.slug)}
      generatedAt={loadParityData().generatedAt}
      harness={found.harness}
    />
  );
}
