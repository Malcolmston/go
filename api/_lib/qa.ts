// qa.ts — shared question/answer memory for the /ask assistant.
//
// Every answered question is recorded in a second Upstash Search index
// ('qa-history'), separate from the 'symbols' index. Before answering a new
// question the assistant can recall semantically-similar past Q&A (via
// qaSearchPast) and use them as HINTS — it still re-grounds against live symbol
// search, so a wrong past answer cannot be recycled as fact.
//
// Uses the same Upstash Search credentials as es.ts (Search.fromEnv reads
// UPSTASH_SEARCH_REST_URL / UPSTASH_SEARCH_REST_TOKEN). The index is created
// implicitly on first upsert — no provisioning needed.

import { Search } from '@upstash/search';
import { createHash } from 'node:crypto';

const INDEX = 'qa-history';

// Content Upstash ranks over (the question, plus a slice of the answer).
interface QaContent extends Record<string, unknown> {
  question: string;
  answer: string;
}

// Everything needed to present a recalled Q&A.
interface QaMeta extends Record<string, unknown> {
  question: string;
  answer: string;
  sources: string; // JSON-encoded {label,url}[] — Upstash metadata values are scalars
  createdAt: string;
}

export interface PastAnswer {
  question: string;
  answer: string;
  sources: { label: string; url: string }[];
  createdAt: string;
  score: number;
}

export interface QaSource {
  label: string;
  url: string;
}

export function qaEnabled(): boolean {
  const url = process.env.UPSTASH_SEARCH_REST_URL;
  const token = process.env.UPSTASH_SEARCH_REST_TOKEN;
  return typeof url === 'string' && url.trim() !== '' && typeof token === 'string' && token.trim() !== '';
}

function makeIndex() {
  return Search.fromEnv().index<QaContent, QaMeta>(INDEX);
}
let handle: ReturnType<typeof makeIndex> | null = null;
function getIndex(): ReturnType<typeof makeIndex> {
  if (!handle) handle = makeIndex();
  return handle;
}

// Upstash rejects content over 4096 serialized chars; keep answers well under.
const ANSWER_MAX = 2500;
function clip(s: string, max: number): string {
  return s.length > max ? s.slice(0, max) : s;
}

// Recall past Q&A most similar to the query. Best-effort: returns [] if the
// index is empty or the service errors, so recall never breaks answering.
export async function qaSearchPast(query: string, limit = 3): Promise<PastAnswer[]> {
  if (!qaEnabled()) return [];
  try {
    const results = await getIndex().search({ query: String(query ?? ''), limit, reranking: true });
    return results.map((r) => {
      const m = r.metadata;
      let sources: QaSource[] = [];
      try {
        sources = m?.sources ? (JSON.parse(m.sources) as QaSource[]) : [];
      } catch {
        sources = [];
      }
      return {
        question: m?.question ?? '',
        answer: m?.answer ?? '',
        sources,
        createdAt: m?.createdAt ?? '',
        score: typeof r.score === 'number' ? r.score : 0,
      };
    });
  } catch {
    return [];
  }
}

// Record an answered question. Keyed by the normalized question so re-asking the
// same thing updates the same entry (latest answer wins) instead of duplicating.
// Best-effort: a storage failure must not fail the user's request.
export async function qaRecord(question: string, answer: string, sources: QaSource[], createdAt: string): Promise<void> {
  if (!qaEnabled()) return;
  const q = String(question ?? '').trim();
  const a = String(answer ?? '').trim();
  if (q === '' || a === '') return;

  const id = 'qa_' + createHash('sha256').update(q.toLowerCase()).digest('hex').slice(0, 24);
  try {
    await getIndex().upsert({
      id,
      content: { question: clip(q, 1000), answer: clip(a, ANSWER_MAX) },
      metadata: {
        question: clip(q, 1000),
        answer: clip(a, ANSWER_MAX),
        sources: JSON.stringify(sources ?? []),
        createdAt,
      },
    });
  } catch {
    // swallow — recording is best-effort
  }
}
