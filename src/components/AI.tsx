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
      <SecH>The site assistant</SecH>
      <p className="muted">The <a className="ask-link" href="/ask">/ask</a> assistant answers questions about the libraries, grounded in a live search over the symbol corpus, and cites in-site deep links. On deployments where it is switched on, it can also <b>run code to verify it</b>: it writes a small Go, Node/TypeScript or Python program and executes it in an isolated, ephemeral sandbox, then reports the real stdout/stderr and exit code before presenting the snippet.</p>
      <p className="muted">It can also read the project&apos;s <b>recorded security findings</b>: ask whether a library has a known issue, whether a particular version is affected, or whether something has been fixed, and the answer comes from the parity security manifests behind the <a className="ask-link" href="/security">Security</a> page — with the affected version range quoted exactly and the fixed/unfixed status derived from the library&apos;s released version, never guessed. It will say plainly when the latest release is still affected, and it does not speculate past what a finding records.</p>
      <div className="note"><b>Code execution is opt-in.</b> It runs only when the deployment sets the <code>CHAT_SANDBOX_ENABLED</code> flag; otherwise the assistant answers normally and simply says it can't run code. The sandbox is ephemeral, receives none of the site's secrets, and is bounded by hard timeouts, output-size caps, and a per-conversation run limit.</div>
      <div className="note"><b>Transparency:</b> AI wrote the code, humans set the direction, and the test suites + protocol interop checks are what make it trustworthy. Treat these as you would any dependency — read the docs, run the tests, pin your versions.</div>
    </section>
  );
}
