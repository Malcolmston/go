import type { DocPackage, DocValue, DocFunc, DocType, DocExample } from '../../docs/types';
import { CodeBlock } from '../CodeBlock';
import { hi } from '../../highlight';
import { DocText } from './DocText';
import { SymbolCard } from './SymbolCard';

export interface PackageViewProps {
  pkg: DocPackage;
}

function valueId(v: DocValue): string {
  return `sym-${v.names[0] ?? 'value'}`;
}

function funcId(fn: DocFunc): string {
  return fn.recv ? `sym-${recvName(fn.recv)}.${fn.name}` : `sym-${fn.name}`;
}

function recvName(recv: string): string {
  return recv.replace(/^\*/, '').replace(/\[.*$/, '');
}

function Example({ ex }: { ex: DocExample }) {
  return (
    <div className="doc-example">
      <div className="doc-example-hd">Example{ex.name ? ` (${ex.name})` : ''}</div>
      {ex.doc ? <DocText text={ex.doc} /> : null}
      <CodeBlock lang="go" html={hi(ex.code)} />
      {ex.output ? (
        <CodeBlock lang="output" text={ex.output} />
      ) : null}
    </div>
  );
}

function Section({ title, count, children }: { title: string; count: number; children: React.ReactNode }) {
  if (count === 0) return null;
  const anchor = `sec-${title.toLowerCase()}`;
  return (
    <section className="doc-sec" id={anchor}>
      <div className="sec-h">
        <span className="bar" />
        <h2>{title}</h2>
        <span className="doc-count">{count}</span>
      </div>
      {children}
    </section>
  );
}

// PackageView renders a single package: its synopsis and doc, then sections for
// Constants, Variables, Types (with their scoped consts/vars/funcs/methods),
// and Functions. Every symbol is deep-linkable via a #sym-<name> anchor.
export function PackageView({ pkg }: PackageViewProps) {
  return (
    <article className="pkg-view">
      <div className="doc-crumb">Package documentation</div>
      <h1 className="pkg-title">
        package {pkg.name}
        {pkg.isCommand ? <span className="pkg-cmd">command</span> : null}
      </h1>
      <div>
        <span className="doc-import">import "{pkg.importPath}"</span>
      </div>

      {pkg.doc ? <DocText text={pkg.doc} /> : <p className="muted">{pkg.synopsis}</p>}

      {pkg.examples && pkg.examples.length > 0 ? (
        <Section title="Examples" count={pkg.examples.length}>
          {pkg.examples.map((ex, i) => (
            <Example key={i} ex={ex} />
          ))}
        </Section>
      ) : null}

      <Section title="Constants" count={pkg.consts.length}>
        {pkg.consts.map((c) => (
          <SymbolCard key={valueId(c)} id={valueId(c)} heading={c.names.join(', ')} signature={c.signature} doc={c.doc} />
        ))}
      </Section>

      <Section title="Variables" count={pkg.vars.length}>
        {pkg.vars.map((v) => (
          <SymbolCard key={valueId(v)} id={valueId(v)} heading={v.names.join(', ')} signature={v.signature} doc={v.doc} />
        ))}
      </Section>

      <Section title="Types" count={pkg.types.length}>
        {pkg.types.map((t) => (
          <TypeCard key={t.name} type={t} />
        ))}
      </Section>

      <Section title="Functions" count={pkg.funcs.length}>
        {pkg.funcs.map((fn) => (
          <SymbolCard key={funcId(fn)} id={funcId(fn)} heading={`func ${fn.name}`} signature={fn.signature} doc={fn.doc} />
        ))}
      </Section>
    </article>
  );
}

function TypeCard({ type }: { type: DocType }) {
  return (
    <SymbolCard id={`sym-${type.name}`} heading={`type ${type.name}`} signature={type.signature} doc={type.doc}>
      {(type.consts.length > 0 || type.vars.length > 0 || type.funcs.length > 0 || type.methods.length > 0) && (
        <div className="sym-members">
          {type.consts.map((c) => (
            <SymbolCard key={valueId(c)} id={valueId(c)} heading={c.names.join(', ')} signature={c.signature} doc={c.doc} />
          ))}
          {type.vars.map((v) => (
            <SymbolCard key={valueId(v)} id={valueId(v)} heading={v.names.join(', ')} signature={v.signature} doc={v.doc} />
          ))}
          {type.funcs.map((fn) => (
            <SymbolCard key={funcId(fn)} id={funcId(fn)} heading={`func ${fn.name}`} signature={fn.signature} doc={fn.doc} />
          ))}
          {type.methods.map((m) => (
            <SymbolCard
              key={funcId(m)}
              id={funcId(m)}
              heading={`func (${m.recv}) ${m.name}`}
              signature={m.signature}
              doc={m.doc}
            />
          ))}
        </div>
      )}
      {type.examples && type.examples.length > 0
        ? type.examples.map((ex, i) => <Example key={i} ex={ex} />)
        : null}
    </SymbolCard>
  );
}
