// /lib/<id> — the library's Overview sub-page. The hero + sub-nav come from the
// shared layout (which also owns generateStaticParams); this page renders only
// the Overview panel.
import { notFound } from 'next/navigation';
import { LIBS } from '../../../src/data';
import { OverviewPanel } from '../../../src/components/lib/OverviewPanel';

export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const lib = LIBS.find((l) => l.id === decodeURIComponent(id));
  if (!lib) notFound();
  return <OverviewPanel lib={lib} />;
}
