// The collapsed-row projection of a harness's nested package ports.
//
// A repo like express carries 28 nested ports, each a full harness against its
// OWN upstream package: cases, symbol inventory, groups, security. That is ~94%
// of the harness JSON, and /parity/<lib> renders none of it until a reader opens
// a disclosure — so the page passes these summaries to <ParityLibrary> (a client
// component, hence everything it receives is serialised into the RSC payload)
// and the bodies are fetched from /parity/<lib>/nested on first open.
//
// This module is deliberately NOT 'use client': the server page calls
// nestedSummaries() while prerendering.

import type { ParityHarness } from './ParityData';

/** Everything the collapsed nested row and its disclosure header show. */
export interface ParityNestedSummary {
  /** The nested harness key, i.e. the nested/<key> directory name. */
  key: string;
  library: string;
  /** upstream.raw of the nested port's OWN pinned oracle. */
  upstream: string;
  goModule: string;
  total: number;
  mismatch: number;
  deviations: number;
  /** coverage.length: inventoried upstream symbols. */
  symbols: number;
  parityPercent: number;
  comparedTotal: number;
}

/** Project a `nested` map down to the rows the collapsed list needs. */
export function nestedSummaries(nested: Record<string, ParityHarness>): ParityNestedSummary[] {
  return Object.entries(nested)
    .map(([key, n]) => ({
      key,
      library: n.library,
      upstream: n.upstream.raw,
      goModule: n.goModule,
      total: n.total,
      mismatch: n.mismatch,
      deviations: n.deviations,
      symbols: n.coverage.length,
      parityPercent: n.parityPercent,
      comparedTotal: n.comparedTotal,
    }))
    .sort((a, b) => a.key.localeCompare(b.key));
}
