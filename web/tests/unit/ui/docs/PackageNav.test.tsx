import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { PackageNav } from 'go-ui';
import type { DocIndex } from 'go-ui';
import sampleJson from '../../../../../ui/src/docs/sample.doc.json';

const sample = sampleJson as unknown as DocIndex;

describe('PackageNav', () => {
  it('lists every package with name and import path', () => {
    render(<PackageNav packages={sample.packages} active={sample.packages[0].importPath} onSelect={() => {}} />);
    expect(screen.getByText('express')).toBeInTheDocument();
    expect(screen.getByText('middleware')).toBeInTheDocument();
    expect(screen.getByText('github.com/malcolmston/express/middleware')).toBeInTheDocument();
  });

  it('marks the active package', () => {
    render(<PackageNav packages={sample.packages} active={sample.packages[1].importPath} onSelect={() => {}} />);
    const active = document.querySelector('.docs-pkg-link.active');
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
});
