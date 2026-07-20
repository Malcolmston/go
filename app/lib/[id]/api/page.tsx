// /lib/<id>/api — the library's API reference sub-page (the full package-by-
// package Go API rendered inline by DocsApp). Hero + sub-nav come from the
// shared layout.
import { notFound } from 'next/navigation';
import { LIBS } from '../../../../src/data';
import { ApiPanel } from '../../../../src/components/lib/ApiPanel';

export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const lib = LIBS.find((l) => l.id === decodeURIComponent(id));
  if (!lib) notFound();
  return <ApiPanel lib={lib} />;
}
