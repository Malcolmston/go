import { LIBS } from '../data';
import { SecH } from './SecH';
import { LibCard } from './LibCard';

export interface HomeProps {
  go: (id: string) => void;
}

// Home is the landing hero + library grid + "why this exists" section.
export function Home({ go }: HomeProps) {
  return (
    <section className="view active" id="view-home">
      <div className="hero">
        <div className="mesh pointer-events-none select-none" />
        <span className="chip"><span className="pulse" /> Standard-library-first · Go 1.23+</span>
        <h1>The Node.js ecosystem,<br />reimagined&nbsp;in&nbsp;Go.</h1>
        <p className="lead">Faithful, idiomatic ports of the libraries you already know — Express, Passport,
          Socket.IO, chalk and morgan from Node.js, plus fastmcp, Streamlit, algebra and opencv from Python —
          built on real wire protocols with (almost) zero third-party dependencies.</p>
        <div className="cta">
          <a className="btn primary" href="#howto" onClick={(e) => { e.preventDefault(); go('howto'); }}>Get started →</a>
          <a className="btn" href="https://github.com/malcolmston" target="_blank" rel="noopener"><i className="fa-brands fa-github" />&nbsp;View on GitHub</a>
        </div>
      </div>

      <div className="bento reveal">
        <div className="stat"><b>9</b><span>drop-in libraries</span></div>
        <div className="stat"><b>300+</b><span>packages &amp; strategies</span></div>
        <div className="stat"><b>~0</b><span>third-party deps</span></div>
        <div className="stat"><b>100%</b><span>Go, wire-compatible</span></div>
      </div>

      <SecH style={{ marginTop: 0 }}>Pick a library</SecH>
      <p className="muted" style={{ marginTop: '-.3rem' }}>Each is a standalone Go module — use one, or wire them together.</p>
      <div className="grid g3" style={{ marginTop: '1.2rem' }}>
        {LIBS.map((l) => <LibCard lib={l} key={l.id} onOpen={go} />)}
      </div>

      <SecH style={{ marginTop: '3rem' }}>Why this exists</SecH>
      <div className="grid g3">
        <div className="card"><span className="ico"><i className="fa-solid fa-bullseye" /></span><h3>Familiar APIs</h3><p className="muted">If you know Express or Passport in Node, you already know these. The same concepts — handlers, middleware, strategies, rooms — mapped to idiomatic Go.</p></div>
        <div className="card"><span className="ico"><i className="fa-solid fa-feather" /></span><h3>Almost no dependencies</h3><p className="muted">Real protocols implemented from scratch on the standard library: RFC 6455 WebSockets, Engine.IO/Socket.IO codecs, JWT/JWKS, ANSI. Only chalk pulls in <code>golang.org/x/term</code>.</p></div>
        <div className="card"><span className="ico"><i className="fa-solid fa-plug" /></span><h3>Interoperable</h3><p className="muted">Socket.IO talks to the real <code>socket.io-client@4</code>. Passport JWTs verify against <code>jsonwebtoken</code>. Express mounts any <code>net/http</code> handler.</p></div>
      </div>
    </section>
  );
}
