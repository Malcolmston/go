import { useRef, useState } from 'react';
import { copyText } from '../clipboard';

export interface CompareCardProps {
  html: string;
  name: string;
  color: string;
}

// CompareCard is a CodeBlock whose header shows a coloured language swatch,
// used in the Node → Go comparison columns.
export function CompareCard({ html, name, color }: CompareCardProps) {
  const codeRef = useRef<HTMLElement>(null);
  const [copied, setCopied] = useState(false);
  const copy = () => {
    const el = codeRef.current;
    if (!el) return;
    copyText(el.innerText);
    // Optimistic feedback (see clipboard.ts — the async API can silently fail).
    setCopied(true);
    setTimeout(() => setCopied(false), 1400);
  };
  return (
    <div className="code">
      <div className="code-hd">
        <span className="dots"><i /><i /><i /></span>
        <span className="lang"><span className="lg"><span className="swatch" style={{ background: color }} />{name}</span></span>
        <button className="copy" onClick={copy}>{copied ? 'Copied' : 'Copy'}</button>
      </div>
      <pre><code ref={codeRef} dangerouslySetInnerHTML={{ __html: html }} /></pre>
    </div>
  );
}
