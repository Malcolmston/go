import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { PackageNav } from 'go-ui';
import type { DocIndex } from 'go-ui';
import sampleJson from '../../../../ui/src/docs/sample.doc.json';

const sample = sampleJson as unknown as DocIndex;

describe('PackageNav', () => {
  it('lists every package with name and import path', () => {
    render(<PackageNav packages={sample.packages} active={sample.packages[0].importPath} onSelect={() => {}} />);
    expect(screen.getByText('express')).toBeInTheDocument();
    expect(screen.getByText('middleware')).toBeInTheDocument();
  });

  it('marks the active package', () => {
    render(<PackageNav packages={sample.packages} active={sample.packages[1].importPath} onSelect={() => {}} />);
    const active = document.querySelector('.gd-pkg-header.open');
    expect(active?.textContent).toContain('middleware');
  });

  it('filters packages by the search query', async () => {
    render(<PackageNav packages={sample.packages} active="" onSelect={() => {}} />);
    await userEvent.type(screen.getByRole('searchbox'), 'middle');
    expect(screen.queryByText('express')).not.toBeInTheDocument();
    expect(screen.getByText('middleware')).toBeInTheDocument();
  });

  it('calls onSelect with the import path when clicked', async () => {
    const onSelect = vi.fn();
    render(<PackageNav packages={sample.packages} active="" onSelect={onSelect} />);
    await userEvent.click(screen.getByText('middleware'));
    expect(onSelect).toHaveBeenCalledWith('github.com/malcolmston/express/middleware');
  });

  it('exposes the expanded package symbols as real links', () => {
    // Regression: the rows carried role="listitem", which stripped the link role
    // off each <a> and hid them from screen-reader link navigation.
    render(
      <PackageNav
        packages={sample.packages}
        active={sample.packages[0].importPath}
        onSelect={() => {}}
      />,
    );
    const link = screen.getByRole('link', { name: /Router$/ });
    expect(link).toHaveAttribute('href', '#sym-Router');
  });

  it('marks the symbol row matching the current hash', () => {
    window.location.hash = '#sym-Router';
    render(
      <PackageNav
        packages={sample.packages}
        active={sample.packages[0].importPath}
        onSelect={() => {}}
      />,
    );
    const active = document.querySelector('.gd-sym-link.active');
    expect(active).toHaveAttribute('aria-current', 'true');
    expect(active?.getAttribute('href')).toBe('#sym-Router');
    window.location.hash = '';
  });

  it('shows an empty state when the filter matches nothing', async () => {
    render(<PackageNav packages={sample.packages} active="" onSelect={() => {}} />);
    await userEvent.type(screen.getByRole('searchbox'), 'zzzznope');
    expect(screen.getByText('No packages match.')).toBeInTheDocument();
  });

  it('renders an empty package list without crashing', () => {
    render(<PackageNav packages={[]} active="" onSelect={() => {}} />);
    expect(screen.getByText('No packages match.')).toBeInTheDocument();
  });

  it('does not crash on a package whose symbol arrays are missing', () => {
    const malformed = [
      { importPath: 'github.com/x/broken', name: 'broken', synopsis: '', doc: '' },
    ] as unknown as DocIndex['packages'];
    expect(() =>
      render(<PackageNav packages={malformed} active="github.com/x/broken" onSelect={() => {}} />),
    ).not.toThrow();
  });
});
