import { SecH } from './SecH';

// AI is the "AI story" tab describing how the suite was built.
export function AI() {
  return (
    <section className="view active" id="view-ai">
      <div className="hero" style={{ padding: '3rem 0 1.5rem' }}>
        <div className="mesh pointer-events-none select-none" style={{ height: 360, opacity: .5 }} />
        <span className="chip"><span className="pulse" /> Built with AI · Ready for AI</span>
        <h1 style={{ fontSize: 'clamp(2rem,5vw,3rem)' }}>The <span className="grad-text">AI</span> story</h1>
        <p className="lead">This entire suite was designed, ported and tested with AI assistance — and it's built to be easy for AI coding agents to consume.</p>
      </div>
      <SecH>How it was built</SecH>
      <p className="muted">Every library here was ported from its Node.js original with <b>Claude (Anthropic)</b> driving the implementation: reading the reference behaviour, writing idiomatic Go, generating table-driven tests, and verifying wire-level compatibility against the real Node clients.</p>
      <div className="grid g3">
        <div className="card"><span className="ico"><i className="fa-solid fa-compass" /></span><h3>Faithful behaviour</h3><p className="muted">Ports were checked against the original semantics — not just "looks right".</p></div>
        <div className="card"><span className="ico"><i className="fa-solid fa-circle-check" /></span><h3>Verified, not vibes</h3><p className="muted">Every package ships tests. CI runs build + vet + test on Go 1.23 and 1.24.</p></div>
        <div className="card"><span className="ico"><i className="fa-solid fa-book" /></span><h3>Self-documenting</h3><p className="muted">A <code>go/doc</code> generator produces the API sites, so docs never drift from the code.</p></div>
      </div>
      <div className="note"><b>Transparency:</b> AI wrote the code, humans set the direction, and the test suites + protocol interop checks are what make it trustworthy. Treat these as you would any dependency — read the docs, run the tests, pin your versions.</div>
    </section>
  );
}
