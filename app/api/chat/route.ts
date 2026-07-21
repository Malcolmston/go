// app/api/chat/route.ts — the /ask assistant backend.
//
// Streams an answer from a chat model (via the Vercel AI Gateway, authenticated
// by VERCEL_OIDC_TOKEN) grounded in the go aggregator's own data. Two tools:
//   - searchSymbols     : hybrid search over the symbol corpus (Upstash Search,
//                         BM25 fallback), returning in-site deep links.
//   - searchPastAnswers : semantically-similar past Q&A from the shared memory,
//                         used as HINTS only (the model re-grounds via search).
//
// Every answered question is recorded back into the shared Q&A memory so the
// assistant improves collectively over time.

import {
  streamText,
  tool,
  stepCountIs,
  convertToModelMessages,
  createUIMessageStreamResponse,
  toUIMessageStream,
  type UIMessage,
} from 'ai';
import { z } from 'zod';
import { esEnabled, esSearch, type SearchHit } from '../../../api/_lib/es';
import { search as bm25Search } from '../../../api/_lib/bm25';
import { getSymbols, type SymbolDoc } from '../../../api/_lib/data';
import { qaSearchPast, qaRecord, type QaSource } from '../../../api/_lib/qa';

const MODEL = 'anthropic/claude-sonnet-4.6';
const CORS_HEADERS: Record<string, string> = {
  'Access-Control-Allow-Origin': '*',
  'Access-Control-Allow-Methods': 'POST, OPTIONS',
  'Access-Control-Allow-Headers': 'Content-Type',
};

// Deep link into the site for a symbol/package. The /ask page runs only where
// server functions exist (Vercel root domain), so plain absolute paths are correct.
function symbolLink(library: unknown, anchor: unknown): string {
  const lib = encodeURIComponent(String(library ?? ''));
  const a = anchor ? `#${String(anchor)}` : '';
  return `/lib/${lib}${a}`;
}

function clip(s: unknown, max: number): string {
  const str = String(s ?? '');
  return str.length > max ? str.slice(0, max) + '…' : str;
}

interface ToolHit {
  name: string;
  kind: string;
  library: string;
  package: string;
  signature: string;
  doc: string;
  url: string;
}

function toToolHit(h: SearchHit | (SymbolDoc & { score: number })): ToolHit {
  return {
    name: String(h.name ?? ''),
    kind: String(h.kind ?? ''),
    library: String(h.library ?? ''),
    package: String(h.packageImportPath ?? ''),
    signature: clip(h.signature, 300),
    doc: clip(h.doc, 500),
    url: symbolLink(h.library, h.anchor),
  };
}

async function runSymbolSearch(query: string, library: string | undefined, limit: number): Promise<ToolHit[]> {
  const want = library ? Math.min(50, limit * 6) : limit;
  let hits: (SearchHit | (SymbolDoc & { score: number }))[] = [];
  if (esEnabled()) {
    try {
      hits = await esSearch(query, want);
    } catch {
      hits = bm25Search(getSymbols(), query, want);
    }
  } else {
    hits = bm25Search(getSymbols(), query, want);
  }
  const scoped = library ? hits.filter((h) => String(h.library ?? '') === library) : hits;
  return scoped.slice(0, limit).map(toToolHit);
}

const SYSTEM = `You are the assistant for "go" (github.com/malcolmston/go), a suite of Go ports of popular Node.js libraries (express, socket.io, passport, cheerio, axios, and more). You help users understand what the libraries do and how to use them, and you point them to exactly where to find things on this site.

Rules:
- ALWAYS call searchSymbols to ground factual answers about packages, functions, types, or "where is X / how do I Y". Never invent symbol names, signatures, or import paths.
- You may call searchPastAnswers to see how similar questions were answered before. Treat those as HINTS ONLY — re-verify against searchSymbols before relying on them. Never repeat a past answer you cannot confirm.
- Cite where to look using the "url" field from searchSymbols results, as Markdown links, e.g. [ParseString](/lib/express#sym-ParseString). Prefer linking the specific symbol; link the library page (/lib/<library>) for broader questions.
- Be concise and practical. Show short Go usage snippets when helpful. If the corpus has nothing relevant, say so plainly rather than guessing.`;

// Extract the latest user question (concatenated text parts) for Q&A recording.
function lastUserText(messages: UIMessage[]): string {
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i];
    if (m.role !== 'user') continue;
    const text = (m.parts ?? [])
      .filter((p): p is { type: 'text'; text: string } => p.type === 'text')
      .map((p) => p.text)
      .join(' ')
      .trim();
    if (text) return text;
  }
  return '';
}

// Pull the /lib deep links the answer actually cited, as sources.
function extractSources(answer: string): QaSource[] {
  const out: QaSource[] = [];
  const seen = new Set<string>();
  const re = /\[([^\]]+)\]\((\/lib\/[^)\s]+)\)/g;
  let match: RegExpExecArray | null;
  while ((match = re.exec(answer)) !== null) {
    const url = match[2];
    if (seen.has(url)) continue;
    seen.add(url);
    out.push({ label: match[1], url });
  }
  return out;
}

export async function OPTIONS(): Promise<Response> {
  return new Response(null, { status: 204, headers: CORS_HEADERS });
}

export async function POST(req: Request): Promise<Response> {
  const { messages }: { messages: UIMessage[] } = await req.json();
  const question = lastUserText(messages);

  const result = streamText({
    model: MODEL,
    system: SYSTEM,
    messages: await convertToModelMessages(messages),
    stopWhen: stepCountIs(6),
    tools: {
      searchSymbols: tool({
        description:
          'Search the go library corpus (functions, types, packages) and return matches with in-site deep links. Use for any factual or "where/how" question.',
        inputSchema: z.object({
          query: z.string().describe('What to search for, e.g. "parse url", "http server listen", "jwt sign".'),
          library: z
            .string()
            .optional()
            .describe('Optional library id to scope results (e.g. "express", "socketio", "passport").'),
        }),
        execute: async ({ query, library }) => runSymbolSearch(query, library, 8),
      }),
      searchPastAnswers: tool({
        description:
          'Recall how similar questions were answered before (shared memory). Hints only — always re-verify with searchSymbols.',
        inputSchema: z.object({
          query: z.string().describe('The user question or topic to find prior answers for.'),
        }),
        execute: async ({ query }) => {
          const past = await qaSearchPast(query, 3);
          return past.map((p) => ({ question: p.question, answer: clip(p.answer, 600), sources: p.sources }));
        },
      }),
    },
    onFinish: async ({ text }) => {
      // Record the Q&A into shared memory (best-effort; never fails the request).
      if (question && text) {
        await qaRecord(question, text, extractSources(text), new Date().toISOString());
      }
    },
  });

  return createUIMessageStreamResponse({
    stream: toUIMessageStream({ stream: result.stream }),
    headers: CORS_HEADERS,
  });
}
