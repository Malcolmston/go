// /parity — the single page for how parity is measured, merged with what used
// to be the separate /pipeline tab. This is a SERVER component: it reads the
// generated dataset from api/_data/parity.json at build time and hands the
// index rows (no per-case or per-symbol bulk) to the client view, so the page
// works identically on Vercel and in the static export.
import type { Metadata } from 'next';
import { Parity } from '../../src/components/Parity';
import { LIBS } from '../../src/data';
import { repoKey } from '../../src/parityLookup';
import { loadParityData, paritySummaries } from './load';

export const metadata: Metadata = {
  title: 'Parity — measured against the original library',
  description:
    'How every Go port is scored: the pinned upstream package it is measured against, the stages of each harness run, and a symbol-by-symbol, case-by-case comparison.',
};

export default function Page() {
  const data = loadParityData();
  const rows = paritySummaries();
  const measured = new Set(Object.keys(data.libraries));
  const unaudited = LIBS.filter((l) => !measured.has(l.id) && !measured.has(repoKey(l))).map((l) => ({
    id: l.id,
    name: l.name,
  }));
  return <Parity generatedAt={data.generatedAt} rows={rows} unaudited={unaudited} />;
}
