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
});
