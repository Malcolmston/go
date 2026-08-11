import Link from 'next/link';
import { LIBS } from '../data';
import { SecH } from './SecH';

// Libraries that are not dependency-free, and what they pull in. Everything not
// listed here is standard-library only.
const EXTRA_DEPS: Record<string, string> = {
  chalk: 'x/term',
};

// About is the "about" tab with the library summary table.
//
// The table is generated from LIBS rather than hand-written: the hand-written
// version listed only 9 of the libraries and had to be edited by hand whenever
// one was added, so it silently fell out of date.
export function About() {
  return (
    <section className="view active" id="view-about">
      <SecH h="h2">About</SecH>
      <p><span className="grad-text" style={{ fontWeight: 700 }}>malcolmston/go</span> is a family of Go libraries that recreate the most-loved building blocks of the Node.js ecosystem — with the same ergonomics, on top of Go&apos;s standard library.</p>
      <SecH>The libraries</SecH>
      <p className="muted">
        All {LIBS.length} modules, what each one ports, and its import path. Every library is an independent Go module —
        depend on one without pulling in the rest.
      </p>
      <div className="about-table-wrap">
        <table className="cmp">
          <caption className="sr-only">Every malcolmston/go library, the upstream project it ports, its module path and its dependencies</caption>
          <thead>
            <tr>
              <th scope="col">Library</th>
              <th scope="col">Ports</th>
              <th scope="col">Ecosystem</th>
              <th scope="col">Module</th>
              <th scope="col">Deps</th>
            </tr>
          </thead>
          <tbody>
            {LIBS.map((lib) => (
              <tr key={lib.id}>
                <th scope="row" style={{ fontWeight: 600 }}>
                  <Link href={`/lib/${lib.id}`}>{lib.name}</Link>
                </th>
                <td className="mono">{lib.node}</td>
                <td>{lib.source ?? 'Node.js'}</td>
                <td className="mono">{lib.pkg}</td>
                <td className={EXTRA_DEPS[lib.id] ? 'mono' : undefined}>{EXTRA_DEPS[lib.id] ?? 'none'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="note">Not affiliated with or endorsed by the original Node.js or Python projects or their authors. Each Go library is an independent re-implementation, released under the MIT License.</div>
      <style>{aboutCss}</style>
    </section>
  );
}

const aboutCss = `
#view-about .about-table-wrap { overflow-x: auto; }
#view-about .cmp th[scope="row"] { text-align: left; }
`;
