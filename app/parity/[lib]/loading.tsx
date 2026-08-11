// Pending UI for /parity/<harness>. This is the heaviest transition on the
// site — the RSC payload for /parity/express is close to a megabyte — and Next
// keeps the PREVIOUS page on screen for the whole of that transfer, so without
// this file a click on a row in /parity looks like a click that did nothing.
//
// A server component, like the page it stands in for. It deliberately renders
// the same `<section className="view active">` wrapper and the same first
// heading level (SecH h="h2") that ParityLibrary renders, so neither the
// heading order nor the page frame jumps when the real payload lands.
//
// The announcement and the drawing are separate jobs: the role="status" line
// carries the text assistive technology reads, and the bars below it are
// decorative and hidden from it entirely.
import { SecH } from '../../../src/components/SecH';

const styles = `
.sk-bar { height: .8rem; border-radius: 6px; background: var(--line, #2a3346); animation: sk-pulse 1.4s ease-in-out infinite; }
.sk-bar + .sk-bar { margin-top: .55rem; }
.sk-rows { margin-top: 1.75rem; display: grid; gap: .9rem; }
.sk-row { display: grid; grid-template-columns: 1fr 4fr 1fr; gap: .9rem; align-items: center; }
.sk-row .sk-bar { height: 1.1rem; }
@keyframes sk-pulse { 0%, 100% { opacity: .55; } 50% { opacity: .2; } }
@media (prefers-reduced-motion: reduce) { .sk-bar { animation: none; } }
`;

export default function Loading() {
  return (
    <section className="view active" id="view-parity-lib-loading">
      <SecH h="h2">Loading the parity record</SecH>
      <p className="muted" role="status" aria-busy="true">
        Fetching the measured run — the pinned upstream oracle, every symbol and every case.
      </p>
      <style>{styles}</style>
      <div className="sk-rows" aria-hidden="true">
        <div className="sk-bar" style={{ width: '60%' }} />
        <div className="sk-bar" style={{ width: '85%' }} />
        {[0, 1, 2, 3, 4, 5].map((i) => (
          <div className="sk-row" key={i}>
            <div className="sk-bar" />
            <div className="sk-bar" />
            <div className="sk-bar" />
          </div>
        ))}
      </div>
    </section>
  );
}
