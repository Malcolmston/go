import { FAQS } from '../data';
import { SecH } from './SecH';
import { Html } from './Html';

// Faq renders the frequently-asked-questions accordion.
export function Faq() {
  return (
    <section className="view active" id="view-faq">
      <SecH h="h2">Frequently asked questions</SecH>
      <div style={{ marginTop: '1rem' }}>
        {FAQS.map(([q, a], i) => (
          <details className="faq" key={i}>
            <summary>{q}</summary>
            <Html tag="div" className="body" html={a} />
          </details>
        ))}
      </div>
    </section>
  );
}
