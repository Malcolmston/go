import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { DocText, parseDoc } from 'go-ui';

describe('DocText', () => {
  const sample =
    'First paragraph mentions https://example.com in prose.\n\n' +
    'Basic usage\n\n' +
    '\tr := express.New()\n\tr.Listen(":3000")';

  it('parses paragraphs, headings and code blocks', () => {
    const blocks = parseDoc(sample);
    expect(blocks.map((b) => b.kind)).toEqual(['para', 'heading', 'code']);
    expect((blocks[2] as { text: string }).text).toContain('r := express.New()');
    expect((blocks[2] as { text: string }).text).not.toMatch(/^\t/);
  });

  it('renders a heading as an h4', () => {
    render(<DocText text={sample} />);
    expect(screen.getByRole('heading', { level: 4, name: 'Basic usage' })).toBeInTheDocument();
  });

  it('linkifies bare URLs into anchors', () => {
    render(<DocText text={sample} />);
    const link = screen.getByRole('link', { name: 'https://example.com' });
    expect(link).toHaveAttribute('href', 'https://example.com');
  });

  it('renders indented spans as a code block', () => {
    const { container } = render(<DocText text={sample} />);
    expect(container.textContent).toContain('r.Listen(":3000")');
    expect(screen.getByText('go')).toBeInTheDocument();
  });

  it('renders nothing for empty text', () => {
    const { container } = render(<DocText text="   " />);
    expect(container.firstChild).toBeNull();
  });

  it('honours an explicit headingLevel so the outline never skips a rank', () => {
    render(<DocText text={sample} headingLevel={2} />);
    expect(screen.getByRole('heading', { level: 2, name: 'Basic usage' })).toBeInTheDocument();
  });

  it('renders a Deprecated: paragraph as a callout', () => {
    const { container } = render(<DocText text="Deprecated: use New instead." />);
    const note = container.querySelector('.gd-note.gd-note-dep');
    expect(note).toBeTruthy();
    expect(note).toHaveTextContent('Deprecated:');
    expect(note).toHaveTextContent('use New instead.');
  });

  it('renders a Note: paragraph as a plain callout', () => {
    const { container } = render(<DocText text="Note: this is safe." />);
    expect(container.querySelector('.gd-note')).toBeTruthy();
    expect(container.querySelector('.gd-note-dep')).toBeNull();
  });

  it('does not linkify non-http schemes', () => {
    const { container } = render(<DocText text="Try javascript:alert(1) or data:text/html,x" />);
    expect(container.querySelector('a')).toBeNull();
  });

  it('escapes highlighted code inside a doc code block', () => {
    const { container } = render(<DocText text={'intro\n\n\ts := "<b>x</b>"'} />);
    expect(container.querySelector('b')).toBeNull();
    expect(container.textContent).toContain('<b>x</b>');
  });

  it('tolerates undefined text', () => {
    const { container } = render(<DocText text={undefined as unknown as string} />);
    expect(container.firstChild).toBeNull();
  });
});
