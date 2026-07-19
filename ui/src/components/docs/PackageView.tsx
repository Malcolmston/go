import type { DocPackage, DocValue, DocFunc, DocType, DocExample } from '../../docs/types';
import { hi } from '../../highlight';
import { DocText } from './DocText';
import { SymbolCard } from './SymbolCard';
import { SummaryTable, isDeprecated } from './SummaryTable';
import type { SummaryRow } from './SummaryTable';

export interface PackageViewProps {
  pkg: DocPackage;
}

// Anchor-id helpers — MUST match the shared scheme so sidebar links and
// summary-table links resolve to the rendered detail cards.
//   value (const/var group): sym-<firstName>
//   func / constructor:       sym-<funcName>
//   type:                     sym-<TypeName>
//   method:                   sym-<recvBase>.<methodName>
function valueId(v: DocValue): string {
  return `sym-${v.names[0] ?? 'value'}`;
}

function funcId(fn: DocFunc): string {
  return fn.recv ? `sym-${recvName(fn.recv)}.${fn.name}` : `sym-${fn.name}`;
}

function recvName(recv: string): string {
  return recv.replace(/^\*/, '').replace(/\[.*$/, '');
}

// firstLine collapses a (possibly multi-line) Go signature to its first line for
// the compact inline signature shown in summary tables.
function firstLine(sig: string): string {
  const line = (sig ?? '').replace(/\r\n?/g, '\n').split('\n')[0].trim();
  return line;
}

function Example({ ex }: { ex: DocExample }) {
  return (
    <div className="gd-detail-example">
      <div className="gd-detail-label">Example{ex.name ? ` (${ex.name})` : ''}</div>
      {ex.doc ? <DocText text={ex.doc} /> : null}
      <pre className="gd-example">
        <code dangerouslySetInnerHTML={{ __html: hi(ex.code) }} />
      </pre>
      {ex.output ? (
        <>
          <div className="gd-detail-label">Output:</div>
          <pre className="gd-example gd-example-out">
            <code>{ex.output}</code>
          </pre>
        </>
      ) : null}
    </div>
  );
}

// valueRows / funcRows / typeRows build summary-table rows preserving the
// anchor-id scheme so each row links to its detail card.
function valueRows(values: DocValue[]): SummaryRow[] {
  return values.map((v) => ({ id: valueId(v), name: v.names.join(', '), doc: v.doc }));
}

function funcRows(funcs: DocFunc[]): SummaryRow[] {
  return funcs.map((fn) => ({
    id: funcId(fn),
    name: fn.name,
    signature: firstLine(fn.signature),
    doc: fn.doc,
  }));
}

function typeRows(types: DocType[]): SummaryRow[] {
  return types.map((t) => ({
    id: `sym-${t.name}`,
    name: t.name,
    signature: firstLine(t.signature),
    doc: t.doc,
  }));
}

// PackageView renders a single Go package in a Javadoc/godoc layout: a package
// label + title with optional command badge, status pills, the package doc,
// then summary tables (Constants / Variables / Types / Functions + per-type
// method summaries), and finally a Detail area of deep-linkable SymbolCards for
// every symbol. Every symbol is anchored via #sym-<...> per the shared scheme.
export function PackageView({ pkg }: PackageViewProps) {
  const hasSummary =
    pkg.consts.length > 0 ||
    pkg.vars.length > 0 ||
    pkg.types.length > 0 ||
    pkg.funcs.length > 0;
  const typesWithFuncs = pkg.types.filter((t) => t.funcs.length > 0 || t.methods.length > 0);
  const pkgExamples = pkg.examples ?? [];

  return (
    <article className="gd-pkg-view">
      <div className="gd-pkg-label">Package {pkg.importPath}</div>
      <h1 className="gd-title" id={`sym-${pkg.name}`}>
        package {pkg.name}
        {pkg.isCommand ? <span className="gd-cmd">command</span> : null}
      </h1>

      <div className="gd-pills">
        <span className="gd-pill gd-pill-info">import "{pkg.importPath}"</span>
        {pkg.isCommand ? <span className="gd-pill gd-pill-ok">command</span> : null}
        {pkgExamples.length > 0 ? (
          <span className="gd-pill gd-pill-info">
            {pkgExamples.length} example{pkgExamples.length === 1 ? '' : 's'}
          </span>
        ) : null}
      </div>

      <DocText text={pkg.doc || pkg.synopsis} />

      {pkgExamples.length > 0 ? (
        <section className="gd-summary">
          <h2 className="gd-section-h">
            Examples
            <span className="gd-count">{pkgExamples.length}</span>
          </h2>
          {pkgExamples.map((ex, i) => (
            <Example key={i} ex={ex} />
          ))}
        </section>
      ) : null}

      {hasSummary ? (
        <>
          <SummaryTable title="Constant Summary" rows={valueRows(pkg.consts)} />
          <SummaryTable title="Variable Summary" rows={valueRows(pkg.vars)} />
          <SummaryTable title="Type Summary" rows={typeRows(pkg.types)} />
          <SummaryTable title="Function Summary" rows={funcRows(pkg.funcs)} />
          {typesWithFuncs.map((t) => (
            <SummaryTable
              key={`sum-${t.name}`}
              title={`type ${t.name} — Methods`}
              rows={[...funcRows(t.funcs), ...funcRows(t.methods)]}
            />
          ))}
        </>
      ) : null}

      <h2 className="gd-section-h gd-detail-h">Detail</h2>

      {pkg.funcs.map((fn) => (
        <SymbolCard
          key={funcId(fn)}
          id={funcId(fn)}
          heading={`func ${fn.name}`}
          signature={fn.signature}
          doc={fn.doc}
          deprecated={isDeprecated(fn.doc)}
        />
      ))}

      {pkg.types.map((t) => (
        <TypeCard key={t.name} type={t} />
      ))}

      {pkg.consts.map((c) => (
        <SymbolCard
          key={valueId(c)}
          id={valueId(c)}
          heading={c.names.join(', ')}
          signature={c.signature}
          doc={c.doc}
          deprecated={isDeprecated(c.doc)}
        />
      ))}

      {pkg.vars.map((v) => (
        <SymbolCard
          key={valueId(v)}
          id={valueId(v)}
          heading={v.names.join(', ')}
          signature={v.signature}
          doc={v.doc}
          deprecated={isDeprecated(v.doc)}
        />
      ))}
    </article>
  );
}

// TypeCard renders a type as a SymbolCard whose children are the nested detail
// cards for its constructors, methods, and type-scoped consts/vars, followed by
// the type's runnable examples.
function TypeCard({ type }: { type: DocType }) {
  const hasMembers =
    type.funcs.length > 0 ||
    type.methods.length > 0 ||
    type.consts.length > 0 ||
    type.vars.length > 0;
  const examples = type.examples ?? [];

  return (
    <SymbolCard
      id={`sym-${type.name}`}
      heading={`type ${type.name}`}
      signature={type.signature}
      doc={type.doc}
      deprecated={isDeprecated(type.doc)}
    >
      {hasMembers ? (
        <div className="gd-members">
          {type.funcs.map((fn) => (
            <SymbolCard
              key={funcId(fn)}
              id={funcId(fn)}
              heading={`func ${fn.name}`}
              signature={fn.signature}
              doc={fn.doc}
              deprecated={isDeprecated(fn.doc)}
            />
          ))}
          {type.methods.map((m) => (
            <SymbolCard
              key={funcId(m)}
              id={funcId(m)}
              heading={`func (${m.recv}) ${m.name}`}
              signature={m.signature}
              doc={m.doc}
              deprecated={isDeprecated(m.doc)}
            />
          ))}
          {type.consts.map((c) => (
            <SymbolCard
              key={valueId(c)}
              id={valueId(c)}
              heading={c.names.join(', ')}
              signature={c.signature}
              doc={c.doc}
              deprecated={isDeprecated(c.doc)}
            />
          ))}
          {type.vars.map((v) => (
            <SymbolCard
              key={valueId(v)}
              id={valueId(v)}
              heading={v.names.join(', ')}
              signature={v.signature}
              doc={v.doc}
              deprecated={isDeprecated(v.doc)}
            />
          ))}
        </div>
      ) : null}
      {examples.map((ex, i) => (
        <Example key={i} ex={ex} />
      ))}
    </SymbolCard>
  );
}
