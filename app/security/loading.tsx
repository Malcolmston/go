// Pending UI for /security. The findings page ships every finding's markdown
// note, so its payload is around 100 KB — long enough on a slow link that the
// transition needs to say something. Same shape as the parity skeleton: the
// real view (src/components/Security.tsx) opens with a `.view active` section
// and an h2, and so does this.
import { SecH } from '../../src/components/SecH';

const styles = `
.sk-bar { height: .8rem; border-radius: 6px; background: var(--line, #2a3346); animation: sk-pulse 1.4s ease-in-out infinite; }
.sk-rows { margin-top: 1.75rem; display: grid; gap: .9rem; }
.sk-row { display: grid; grid-template-columns: 1fr 3fr 1fr; gap: .9rem; align-items: center; }
.sk-row .sk-bar { height: 1.1rem; }
@keyframes sk-pulse { 0%, 100% { opacity: .55; } 50% { opacity: .2; } }
@media (prefers-reduced-motion: reduce) { .sk-bar { animation: none; } }
`;

export default function Loading() {
  return (
    <section className="view active" id="view-security-loading">
      <SecH h="h2">Loading the security findings</SecH>
      <p className="muted" role="status" aria-busy="true">
        Fetching the disclosure record: severity, affected range and the parity cases that
        detected each finding.
      </p>
      <style>{styles}</style>
      <div className="sk-rows" aria-hidden="true">
        <div className="sk-bar" style={{ width: '70%' }} />
        {[0, 1, 2, 3, 4].map((i) => (
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
