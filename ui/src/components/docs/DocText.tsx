import type { ReactNode } from 'react';
import { CodeBlock } from '../CodeBlock';
import { hi } from '../../highlight';

export interface DocTextProps {
  text: string;
}

type Block =
  | { kind: 'para'; text: string }
  | { kind: 'heading'; text: string }
  | { kind: 'code'; text: string };

const URL_RE = /(https?:\/\/[^\s<>()]+[^\s<>().,;:!?])/g;

// Callout prefixes. A paragraph opening with one of these is rendered as a
// `.gd-note` callout instead of a plain paragraph. "Deprecated:" additionally
// gets the red-tinted `gd-note-dep` modifier.
const NOTE_RE = /^(Note|NOTE):\s*/;
const DEP_RE = /^(Deprecated):\s*/;

function isIndented(line: string): boolean {
  return line.startsWith('\t') || line.startsWith('    ');
}

function dedent(line: string): string {
  if (line.startsWith('\t')) return line.slice(1);
  if (line.startsWith('    ')) return line.slice(4);
  return line;
}

// parseDoc converts a Go doc comment into a small set of blocks. It follows the
// classic go/doc conventions: blank lines separate paragraphs, indented spans
// are pre-formatted code, and a lone unindented line that reads like a title
// (starts uppercase, no trailing punctuation) becomes a heading.
export function parseDoc(text: string): Block[] {
  const lines = (text ?? '').replace(/\r\n?/g, '\n').split('\n');
  const blocks: Block[] = [];
  let i = 0;
  while (i < lines.length) {
    if (lines[i].trim() === '') {
      i++;
      continue;
    }
    if (isIndented(lines[i])) {
      const code: string[] = [];
      while (i < lines.length && (isIndented(lines[i]) || lines[i].trim() === '')) {
        // stop trailing blanks from being swallowed into the code block
        if (lines[i].trim() === '' && !(i + 1 < lines.length && isIndented(lines[i + 1]))) break;
        code.push(dedent(lines[i]));
        i++;
      }
      blocks.push({ kind: 'code', text: code.join('\n').replace(/\n+$/, '') });
      continue;
    }
    const para: string[] = [];
    while (i < lines.length && lines[i].trim() !== '' && !isIndented(lines[i])) {
      para.push(lines[i].trim());
      i++;
    }
    const joined = para.join(' ');
    if (para.length === 1 && isHeading(joined)) {
      blocks.push({ kind: 'heading', text: joined });
    } else {
      blocks.push({ kind: 'para', text: joined });
    }
  }
  return blocks;
}

function isHeading(line: string): boolean {
  if (line.length === 0 || line.length > 72) return false;
  if (!/^[A-Z]/.test(line)) return false;
  if (/[.,:;!?]$/.test(line)) return false;
  return true;
}

// linkify splits a paragraph into text and anchor nodes for bare URLs.
function linkify(text: string): ReactNode[] {
  const out: ReactNode[] = [];
  let last = 0;
  let key = 0;
  text.replace(URL_RE, (url, _g1, offset: number) => {
    if (offset > last) out.push(text.slice(last, offset));
    out.push(
      <a key={key++} href={url} target="_blank" rel="noopener noreferrer">
        {url}
      </a>,
    );
    last = offset + url.length;
    return url;
  });
  if (last < text.length) out.push(text.slice(last));
  return out.length ? out : [text];
}

// renderPara renders a plain paragraph, promoting recognised "Note:" /
// "Deprecated:" leaders into `.gd-note` callouts. URLs stay linkified in every
// variant.
function renderPara(text: string, key: number): ReactNode {
  const dep = DEP_RE.exec(text);
  if (dep) {
    const rest = text.slice(dep[0].length);
    return (
      <div key={key} className="gd-note gd-note-dep">
        <strong>Deprecated:</strong> {linkify(rest)}
      </div>
    );
  }
  const note = NOTE_RE.exec(text);
  if (note) {
    const rest = text.slice(note[0].length);
    return (
      <div key={key} className="gd-note">
        <strong>Note:</strong> {linkify(rest)}
      </div>
    );
  }
  return (
    <p key={key}>{linkify(text)}</p>
  );
}

// DocText renders Go doc-comment text to HTML: paragraphs, indented code blocks
// (highlighted via the shared CodeBlock), simple headings, linkified URLs, and
// "Note:"/"Deprecated:" callouts. It is intentionally dependency-free.
export function DocText({ text }: DocTextProps) {
  const trimmed = (text ?? '').trim();
  if (!trimmed) return null;
  const blocks = parseDoc(text);
  return (
    <div className="gd-prose">
      {blocks.map((b, idx) => {
        if (b.kind === 'code') {
          return <CodeBlock key={idx} lang="go" html={hi(b.text)} />;
        }
        if (b.kind === 'heading') {
          return (
            <h4 key={idx}>{b.text}</h4>
          );
        }
        return renderPara(b.text, idx);
      })}
    </div>
  );
}
