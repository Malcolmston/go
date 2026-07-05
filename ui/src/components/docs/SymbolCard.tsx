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
  // Optional deeper nesting (methods/consts) rendered inside a type card.
  children?: React.ReactNode;
}

// SymbolCard renders one API symbol: an anchored heading, its highlighted
// signature, and its documentation. Reused for consts, vars, funcs, and types.
export function SymbolCard({ id, heading, signature, doc, children }: SymbolCardProps) {
  return (
    <section className="sym-card" id={id}>
      <h3 className="sym-name">
        <a className="sym-anchor" href={`#${id}`} aria-label={`Link to ${heading}`}>
          #
        </a>
        {heading}
      </h3>
      <CodeBlock lang="go" html={hi(signature)} />
      {doc ? <DocText text={doc} /> : null}
      {children}
    </section>
  );
}
