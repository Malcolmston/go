// /lib/<id>/examples — the library's Examples sub-page (Node.js → Go comparison
// and a fuller integration snippet). Hero + sub-nav come from the shared layout.
import { notFound } from 'next/navigation';
import { LIBS } from '../../../../src/data';
import { ExamplesPanel } from '../../../../src/components/lib/ExamplesPanel';

export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const lib = LIBS.find((l) => l.id === decodeURIComponent(id));
  if (!lib) notFound();
  return <ExamplesPanel lib={lib} />;
}
