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
});
