import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/react';
import { JsonLd, serializeJsonLd } from '../../../src/components/JsonLd';

// The component half of the structured-data plumbing: the payload has to land in
// a <script type="application/ld+json"> whose text parses back to the object,
// escaped so a value containing markup cannot close the element early.
describe('JsonLd', () => {
  it('renders a script of type application/ld+json holding the payload', () => {
    const data = { '@context': 'https://schema.org', '@type': 'WebSite', name: 'x' };
    const { container } = render(<JsonLd id="ld-test" data={data} />);
    const script = container.querySelector('script#ld-test');
    expect(script).not.toBeNull();
    expect(script!.getAttribute('type')).toBe('application/ld+json');
    expect(JSON.parse(script!.textContent ?? '')).toEqual(data);
  });

  it('does not emit a literal </script> from a string value', () => {
    const { container } = render(<JsonLd data={{ text: '</script><b>hi</b>' }} />);
    const script = container.querySelector('script')!;
    // One script element only: the payload did not terminate it early.
    expect(container.querySelectorAll('script')).toHaveLength(1);
    expect(container.querySelector('b')).toBeNull();
    expect(script.textContent).toBe(serializeJsonLd({ text: '</script><b>hi</b>' }));
    expect(JSON.parse(script.textContent ?? '').text).toBe('</script><b>hi</b>');
  });

  it('omits the id attribute when none is given', () => {
    const { container } = render(<JsonLd data={{ a: 1 }} />);
    expect(container.querySelector('script')!.hasAttribute('id')).toBe(false);
  });
});
