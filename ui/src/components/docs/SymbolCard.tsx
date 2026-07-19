import { CodeBlock } from '../CodeBlock';
import { hi } from '../../highlight';
import { DocText } from './DocText';

export interface SymbolCardProps {
  // Anchor id used for deep-linking, e.g. "sym-Router" or "sym-Router.Get".
  id: string;
  // Display name / kind label shown in the card header, e.g. "func New".
  heading: string;
  // Go source signature; highlighted via hi() and shown in a CodeBlock.
  signature: string;
  // Doc-comment text for the symbol (rendered by DocText).
  doc?: string;
  // When true, mark the symbol with an inline red "Deprecated" badge.
  deprecated?: boolean;
  // Optional deeper nesting (methods/consts) rendered inside a type card.
  children?: React.ReactNode;
}

// SymbolCard renders one Javadoc-style "detail" entry: an anchored monospace
// heading, its highlighted signature (plain, no left accent), the symbol's
// documentation, and any nested members/examples. Reused for consts, vars,
// funcs, types, and methods.
export function SymbolCard({ id, heading, signature, doc, deprecated, children }: SymbolCardProps) {
  return (
    <section className="gd-detail">
      <h3 className="gd-detail-name" id={id}>
        {heading}
        {deprecated ? <span className="gd-dep-badge">Deprecated</span> : null}
      </h3>
      <div className="gd-sig-plain">
        <CodeBlock lang="go" html={hi(signature)} />
      </div>
      {doc ? <DocText text={doc} /> : null}
      {children}
    </section>
  );
}
