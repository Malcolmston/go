import { describe, it, expect } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { About } from '../../../src/components/About';
import { LIBS } from '../../../src/data';

describe('About', () => {
  it('renders the about heading and the library summary table', () => {
    const { container } = render(<About />);
    expect(container.querySelector('#view-about')).not.toBeNull();
    expect(screen.getByRole('heading', { level: 2, name: 'About' })).toBeInTheDocument();
    expect(screen.getByRole('table')).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Library' })).toBeInTheDocument();
    // Each library is a row header (th scope="row") linking to its page.
    expect(screen.getByRole('rowheader', { name: 'Express' })).toBeInTheDocument();
  });

  it('lists every library, not just a hand-picked few', () => {
    render(<About />);
    const body = screen.getByRole('table').querySelector('tbody');
    expect(body).not.toBeNull();
    // one row per library
    expect(within(body as HTMLElement).getAllByRole('row').length).toBe(LIBS.length);
    for (const lib of LIBS) {
      expect(screen.getByRole('rowheader', { name: lib.name })).toBeInTheDocument();
    }
  });

  it('links each library row to its route', () => {
    render(<About />);
    for (const lib of LIBS.slice(0, 5)) {
      expect(screen.getByRole('link', { name: lib.name })).toHaveAttribute('href', `/lib/${lib.id}`);
    }
  });

  it('marks chalk as the only library with a third-party dependency', () => {
    render(<About />);
    const rows = screen.getAllByRole('row');
    const chalkRow = rows.find((r) => within(r).queryByRole('rowheader', { name: 'chalk' }));
    expect(chalkRow).toBeDefined();
    expect(within(chalkRow as HTMLElement).getByText('x/term')).toBeInTheDocument();
  });

  it('labels each library with the ecosystem it ports from', () => {
    render(<About />);
    const rows = screen.getAllByRole('row');
    const streamlit = rows.find((r) => within(r).queryByRole('rowheader', { name: 'Streamlit' }));
    expect(streamlit).toBeDefined();
    expect(within(streamlit as HTMLElement).getByText('Python')).toBeInTheDocument();
  });
});
