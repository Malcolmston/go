'use client';

import { useChat } from '@ai-sdk/react';
import { useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { SecH } from './SecH';

// Ask — the AI assistant tab. Chats with /api/chat, which grounds answers in
// the symbol corpus (Upstash Search) and cites in-site deep links. Answers and
// the links they cite are surfaced so users can jump straight to the source.

interface ToolHit {
  name: string;
  kind: string;
  library: string;
  package: string;
  url: string;
}

const SUGGESTIONS = [
  'How do I parse a URL in express?',
  'Which library handles websockets?',
  'How do I sign a JWT?',
  'Where is the HTML parser?',
];

// Render an assistant message as Markdown (GFM: tables, code, lists, links).
// Links get the ask-link style; external ones open in a new tab.
function AnswerMarkdown({ text }: { text: string }) {
  return (
    <div className="ask-md">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          a: ({ href, children }) => {
            const url = href ?? '';
            const external = /^https?:\/\//.test(url);
            return (
              <a
                className="ask-link"
                href={url}
                {...(external ? { target: '_blank', rel: 'noopener noreferrer' } : {})}
              >
                {children}
              </a>
            );
          },
        }}
      >
        {text}
      </ReactMarkdown>
    </div>
  );
}

// Collect the searchSymbols results a message produced, deduped by url.
function messageSources(parts: readonly { type: string; state?: string; output?: unknown }[]): ToolHit[] {
  const out: ToolHit[] = [];
  const seen = new Set<string>();
  for (const p of parts) {
    if (p.type !== 'tool-searchSymbols' || p.state !== 'output-available') continue;
    const hits = Array.isArray(p.output) ? (p.output as ToolHit[]) : [];
    for (const h of hits) {
      if (!h?.url || seen.has(h.url)) continue;
      seen.add(h.url);
      out.push(h);
    }
  }
  return out;
}

export function Ask() {
  const [input, setInput] = useState('');
  const { messages, sendMessage, status } = useChat();
  const busy = status === 'submitted' || status === 'streaming';

  const send = (text: string) => {
    const t = text.trim();
    if (!t || busy) return;
    sendMessage({ text: t });
    setInput('');
  };

  return (
    <section className="view active" id="view-ask">
      <div className="hero" style={{ padding: '2.5rem 0 1rem' }}>
        <span className="chip">
          <span className="pulse" /> AI assistant · grounded in the API
        </span>
        <h1 style={{ fontSize: 'clamp(1.8rem,4.5vw,2.6rem)' }}>
          Ask the <span className="grad-text">go</span> assistant
        </h1>
        <p className="lead">
          Ask what a library does or how to use it. Answers are grounded in the live symbol search and link straight to
          the relevant page.
        </p>
      </div>

      <SecH>Conversation</SecH>

      <div className="ask-chat card" style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
        {messages.length === 0 && (
          <div className="ask-empty muted">
            <p>Try one of these, or ask your own:</p>
            <div className="ask-suggest">
              {SUGGESTIONS.map((s) => (
                <button key={s} className="pill" onClick={() => send(s)} disabled={busy}>
                  {s}
                </button>
              ))}
            </div>
          </div>
        )}

        {messages.map((message) => {
          const isUser = message.role === 'user';
          const sources = !isUser ? messageSources(message.parts as never) : [];
          return (
            <div key={message.id} className={`ask-msg ask-msg-${isUser ? 'user' : 'ai'}`}>
              <div className="ask-role muted">{isUser ? 'You' : 'Assistant'}</div>
              <div className="ask-body">
                {message.parts.map((part, i) => {
                  if (part.type === 'text') {
                    return <AnswerMarkdown key={`${message.id}-${i}`} text={part.text} />;
                  }
                  if (part.type === 'tool-searchSymbols' && part.state !== 'output-available') {
                    return (
                      <div key={`${message.id}-${i}`} className="ask-tool muted">
                        <i className="fa-solid fa-magnifying-glass" /> searching the API…
                      </div>
                    );
                  }
                  if (part.type === 'tool-searchPastAnswers' && part.state !== 'output-available') {
                    return (
                      <div key={`${message.id}-${i}`} className="ask-tool muted">
                        <i className="fa-solid fa-clock-rotate-left" /> recalling past answers…
                      </div>
                    );
                  }
                  return null;
                })}

                {sources.length > 0 && (
                  <div className="ask-sources">
                    <div className="ask-sources-h muted">Where to look</div>
                    <ul>
                      {sources.map((h) => (
                        <li key={h.url}>
                          <a className="ask-link" href={h.url}>
                            {h.name}
                          </a>{' '}
                          <span className="muted">
                            {h.kind} · {h.library}
                          </span>
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
              </div>
            </div>
          );
        })}

        {busy && messages[messages.length - 1]?.role === 'user' && (
          <div className="ask-msg ask-msg-ai">
            <div className="ask-role muted">Assistant</div>
            <div className="ask-body muted">
              <span className="pulse" /> thinking…
            </div>
          </div>
        )}
      </div>

      <form
        className="ask-form"
        onSubmit={(e) => {
          e.preventDefault();
          send(input);
        }}
      >
        <input
          className="ask-input"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="Ask how to do something, or where to find it…"
          aria-label="Ask the go assistant"
        />
        <button className="btn primary" type="submit" disabled={busy || input.trim() === ''}>
          {busy ? 'Sending…' : 'Send'}
        </button>
      </form>

      <p className="muted ask-disclaimer">
        Answers are AI-generated from this project's own API index and may be imperfect — follow the links and verify
        against the docs and tests.
      </p>
    </section>
  );
}
