import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { PackageView } from 'go-ui';
import type { DocIndex } from 'go-ui';
import sampleJson from '../../../../ui/src/docs/sample.doc.json';

const sample = sampleJson as unknown as DocIndex;
const express = sample.packages[0];

describe('PackageView', () => {
  it('renders the package title and import path', () => {
    render(<PackageView pkg={express} />);
    expect(screen.getByRole('heading', { level: 1, name: /package express/ })).toBeInTheDocument();
    expect(screen.getByText('import "github.com/malcolmston/express"')).toBeInTheDocument();
  });

  it('renders summary section headings for consts, vars, types and funcs', () => {
    render(<PackageView pkg={express} />);
    // Javadoc-style summary headings carry a trailing count, so match by prefix.
    expect(screen.getByRole('heading', { level: 2, name: /Constant Summary/ })).toBeInTheDocument();
    expect(screen.getByRole('heading', { level: 2, name: /Variable Summary/ })).toBeInTheDocument();
    expect(screen.getByRole('heading', { level: 2, name: /Type Summary/ })).toBeInTheDocument();
    expect(screen.getByRole('heading', { level: 2, name: /Function Summary/ })).toBeInTheDocument();
  });

  it('renders types with their nested methods and constructor funcs', () => {
    const { container } = render(<PackageView pkg={express} />);
    // type card anchor
    expect(container.querySelector('#sym-Router')).not.toBeNull();
    // constructor func nested under the type
    expect(container.querySelector('#sym-New')).not.toBeNull();
    // method anchor uses Type.Method form
    expect(container.querySelector('#sym-Router\\.Get')).not.toBeNull();
    expect(screen.getByRole('heading', { name: /func \(\*Router\) Get/ })).toBeInTheDocument();
  });

  it('renders a package-level function symbol', () => {
    const { container } = render(<PackageView pkg={express} />);
    expect(container.querySelector('#sym-Static')).not.toBeNull();
    // The doc appears in both the summary table and the detail card, so assert
    // on the container text rather than a unique element.
    expect(container.textContent).toContain('Static returns a handler');
  });

  it('renders an example with its expected output', () => {
    const { container } = render(<PackageView pkg={express} />);
    expect(container.textContent).toContain('listening on :3000');
  });

  it('keeps a strictly descending heading outline (h1 → h2 → h3 → h4)', () => {
    const { container } = render(<PackageView pkg={express} />);
    const levels = Array.from(container.querySelectorAll('h1,h2,h3,h4,h5')).map((h) =>
      Number(h.tagName.slice(1)),
    );
    expect(levels[0]).toBe(1);
    for (let i = 1; i < levels.length; i++) {
      // A heading may never skip a level going deeper.
      expect(levels[i] - levels[i - 1]).toBeLessThanOrEqual(1);
    }
  });

  it('renders nested type members one level deeper than the type card', () => {
    const { container } = render(<PackageView pkg={express} />);
    expect(container.querySelector('#sym-Router')?.tagName).toBe('H3');
    expect(container.querySelector('#sym-Router\\.Get')?.tagName).toBe('H4');
  });

  it('renders a package with no symbols as an empty page rather than crashing', () => {
    const bare = {
      importPath: 'github.com/x/empty',
      name: 'empty',
      synopsis: 'nothing here',
      doc: '',
      consts: [],
      vars: [],
      types: [],
      funcs: [],
    };
    render(<PackageView pkg={bare} />);
    expect(screen.getByRole('heading', { level: 1, name: /package empty/ })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Detail' })).not.toBeInTheDocument();
  });

  it('tolerates a malformed package whose symbol arrays are missing', () => {
    // A hand-written / truncated doc.json must degrade, not throw.
    const malformed = {
      importPath: 'github.com/x/broken',
      name: 'broken',
      synopsis: '',
      doc: '',
    } as unknown as Parameters<typeof PackageView>[0]['pkg'];
    expect(() => render(<PackageView pkg={malformed} />)).not.toThrow();
    expect(screen.getByRole('heading', { level: 1, name: /package broken/ })).toBeInTheDocument();
  });
});
