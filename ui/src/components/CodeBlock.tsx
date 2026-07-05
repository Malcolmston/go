import { useRef, useState } from 'react';
import { copyText } from '../clipboard';

export interface CodeBlockProps {
  html?: string;
  text?: string;
  lang?: string;
}

// CodeBlock renders a glass "code card". `html` is pre-highlighted markup
// (from hi()); pass plain text via `text` for shell snippets.
export function CodeBlock({ html, text, lang = 'go' }: CodeBlockProps) {
  const codeRef = useRef<HTMLElement>(null);
  const [copied, setCopied] = useState(false);
  const copy = () => {
    const el = codeRef.current;
    if (!el) return;
    copyText(el.innerText);
    // Optimistic feedback: show "Copied" regardless of whether the async
    // Clipboard API resolves (it is focus/permission-gated in some browsers).
    setCopied(true);
    setTimeout(() => setCopied(false), 1400);
  };
  return (
    <div className="code">
      <div className="code-hd">
        <span className="dots"><i /><i /><i /></span>
        <span className="lang">{lang}</span>
        <button className="copy" onClick={copy}>{copied ? 'Copied' : 'Copy'}</button>
      </div>
      <pre>
        <code ref={codeRef} dangerouslySetInnerHTML={html ? { __html: html } : undefined}>
          {html ? undefined : text}
        </code>
      </pre>
    </div>
  );
}
