// analytics.ts — best-effort, structured search analytics.
//
// logSearch() emits ONE structured JSON line per search via console.log. On
// Vercel these lines are captured automatically by the platform's log pipeline
// (visible in the dashboard's Logs view and forwardable to any external sink via
// a Log Drain). No database or extra request is involved — analytics are derived
// entirely from these log lines, which keeps the search path fast and dependency
// free. Because it is purely observational it must never affect the response:
// every call is wrapped in try/catch and swallows its own errors.
//
// Downstream queries built on this include popular-query and zero-result
// tracking (the `zero` flag), backend mix (upstash vs memory), and latency.

export interface SearchEvent {
  q: string;
  first: number;
  library: string;
  kind: string[];
  hits: number;
  backend: string;
  tookMs: number;
  reranked: boolean;
}

// Emit a single-line JSON analytics record. Best-effort: never throws, never
// blocks the response.
export function logSearch(event: SearchEvent): void {
  try {
    console.log(
      JSON.stringify({
        evt: 'search',
        q: event.q,
        first: event.first,
        library: event.library,
        kind: event.kind,
        hits: event.hits,
        backend: event.backend,
        tookMs: event.tookMs,
        reranked: event.reranked,
        zero: event.hits === 0,
      }),
    );
  } catch {
    // Analytics are non-critical — never let a logging failure surface.
  }
}
