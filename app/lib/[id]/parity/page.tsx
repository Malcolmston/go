// /lib/<id>/parity — the library's Parity sub-page (live upstream-parity score
// tiles, or a note when unaudited). Hero + sub-nav come from the shared layout.
import { notFound } from 'next/navigation';
import { LIBS } from '../../../../src/data';
import { ParityPanel } from '../../../../src/components/lib/ParityPanel';

export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const lib = LIBS.find((l) => l.id === decodeURIComponent(id));
  if (!lib) notFound();
  return <ParityPanel lib={lib} />;
}
