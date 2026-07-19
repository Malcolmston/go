// SummaryTable renders a Javadoc/godoc-style "summary" table: one row per symbol
// with a monospace name that links to its detail anchor (#sym-...) and a
// description cell holding an optional inline signature line plus the first
// sentence of the symbol's doc. Zebra striping is applied to odd rows via
// gd-row-alt, and symbols whose doc begins a line with "Deprecated:" get an
// inline gd-dep-badge. It is used by PackageView for the Constants, Variables,
// Type, Function, and per-type Method summaries.

export interface SummaryRow {
  // Anchor id (without the leading '#'), e.g. "sym-Router" or "sym-Router.Get".
  id: string;
  // Monospace display name shown in the first column, e.g. "New" or "(*LRU) Get".
  name: string;
  // Optional short signature shown on its own line in the description cell.
  signature?: string;
  // Doc-comment text; its first sentence becomes the description.
  doc?: string;
}

export interface SummaryTableProps {
  // Section heading rendered above the table, e.g. "Function Summary".
  title: string;
  rows: SummaryRow[];
}

// isDeprecated reports whether a doc comment carries a "Deprecated:" line, per
// the Go convention (a paragraph beginning with that marker).
export function isDeprecated(doc?: string): boolean {
  return !!doc && /^Deprecated:/m.test(doc);
}

// firstSentence extracts the leading sentence of a doc comment for use as a
// one-line summary, mirroring go/doc's Synopsis: it collapses the first
// paragraph to a single line and stops at the first sentence-ending period.
export function firstSentence(doc?: string): string {
  if (!doc) return '';
  const text = doc.replace(/\r\n?/g, '\n').trim();
  if (!text) return '';
  const firstPara = text.split(/\n[ \t]*\n/)[0].replace(/\s+/g, ' ').trim();
  const m = firstPara.match(/^(.*?[.!?])(\s|$)/);
  return m ? m[1] : firstPara;
}

export function SummaryTable({ title, rows }: SummaryTableProps) {
  if (rows.length === 0) return null;
  return (
    <section className="gd-summary">
      <h2 className="gd-section-h">
        {title}
        <span className="gd-count">{rows.length}</span>
      </h2>
      <table className="gd-table">
        <thead>
          <tr>
            <th scope="col">Name</th>
            <th scope="col">Description</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row, i) => {
            const dep = isDeprecated(row.doc);
            const summary = firstSentence(row.doc);
            return (
              <tr key={`${row.id}-${i}`} className={i % 2 === 1 ? 'gd-row-alt' : undefined}>
                <td>
                  <a href={`#${row.id}`}>{row.name}</a>
                  {dep ? <span className="gd-dep-badge">Deprecated</span> : null}
                </td>
                <td>
                  {row.signature ? <div className="gd-sig-inline">{row.signature}</div> : null}
                  {summary ? <span>{summary}</span> : null}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </section>
  );
}
