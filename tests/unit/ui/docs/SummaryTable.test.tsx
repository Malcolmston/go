import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { SummaryTable, isDeprecated, firstSentence } from 'go-ui';
import type { SummaryRow } from 'go-ui';

const rows: SummaryRow[] = [
  { id: 'sym-New', name: 'New', signature: 'func New() *Router', doc: 'New builds a router. It allocates.' },
  { id: 'sym-Old', name: 'Old', doc: 'Old does things.\n\nDeprecated: use New.' },
];

describe('firstSentence', () => {
  it('stops at the first sentence terminator', () => {
    expect(firstSentence('New builds a router. It allocates.')).toBe('New builds a router.');
  });

  it('collapses whitespace within the first paragraph', () => {
    expect(firstSentence('A\n  multi   line\nlead in')).toBe('A multi line lead in');
  });

  it('returns an empty string for missing docs', () => {
    expect(firstSentence(undefined)).toBe('');
    expect(firstSentence('   ')).toBe('');
  });
});

describe('isDeprecated', () => {
  it('detects a Deprecated: paragraph anywhere in the doc', () => {
    expect(isDeprecated('Old does things.\n\nDeprecated: use New.')).toBe(true);
  });

  it('is false for prose that merely mentions deprecation', () => {
    expect(isDeprecated('This is not deprecated: really.')).toBe(false);
    expect(isDeprecated(undefined)).toBe(false);
  });
});

describe('SummaryTable', () => {
  it('renders nothing when there are no rows', () => {
    const { container } = render(<SummaryTable title="Function Summary" rows={[]} />);
    expect(container).toBeEmptyDOMElement();
  });

  it('renders a heading with the row count and links each row to its anchor', () => {
    render(<SummaryTable title="Function Summary" rows={rows} />);
    expect(screen.getByRole('heading', { level: 2 })).toHaveTextContent('Function Summary');
    expect(screen.getByRole('heading', { level: 2 })).toHaveTextContent('2');
    expect(screen.getByRole('link', { name: 'New' })).toHaveAttribute('href', '#sym-New');
  });

  it('shows the inline signature and the first sentence of the doc', () => {
    render(<SummaryTable title="Function Summary" rows={rows} />);
    expect(screen.getByText('func New() *Router')).toBeInTheDocument();
    expect(screen.getByText('New builds a router.')).toBeInTheDocument();
  });

  it('badges deprecated rows', () => {
    render(<SummaryTable title="Function Summary" rows={rows} />);
    const badges = screen.getAllByText('Deprecated');
    expect(badges).toHaveLength(1);
  });

  it('zebra-stripes odd rows', () => {
    const { container } = render(<SummaryTable title="Function Summary" rows={rows} />);
    const bodyRows = container.querySelectorAll('tbody tr');
    expect(bodyRows[0].className).not.toContain('gd-row-alt');
    expect(bodyRows[1].className).toContain('gd-row-alt');
  });
});
