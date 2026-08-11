import { describe, it, expect } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ParityLibrary } from '../../../src/components/ParityLibrary';
import { harnessFixture, parityFixture } from './parityFixture';

const harness = harnessFixture('express');

function renderRecord() {
  return render(
    <ParityLibrary
      slug="express"
      name="Express"
      accent="#38bdf8"
      repo="https://github.com/malcolmston/express"
      libRoute="/lib/express"
      generatedAt={parityFixture().generatedAt}
      harness={harness}
    />,
  );
}

describe('ParityLibrary', () => {
  it('names the exact oracle and the exact module measured', () => {
    renderRecord();
    expect(
      screen.getByRole('heading', { level: 2, name: /Express measured against express@4.19.2/ }),
    ).toBeInTheDocument();
    // The canvas and its textual equivalent both name the artefacts, so these
    // strings legitimately appear more than once.
    expect(screen.getAllByText('4.19.2').length).toBeGreaterThan(0);
    expect(screen.getAllByText('github.com/malcolmston/express').length).toBeGreaterThan(0);
    expect(screen.getAllByText('parity/express/node/run.js').length).toBeGreaterThan(0);
    expect(screen.getAllByText('parity/express/go/run.go').length).toBeGreaterThan(0);
  });

  it('describes the stages of this run with its own artefacts', () => {
    renderRecord();
    expect(screen.getByRole('heading', { name: 'The stages this run executed' })).toBeInTheDocument();
    expect(screen.getAllByText('npm install --save-exact express@4.19.2').length).toBeGreaterThan(0);
    expect(screen.getAllByText('parity/express/cases/*.json').length).toBeGreaterThan(0);
  });

  it('shows every upstream symbol with its Go counterpart and status', () => {
    renderRecord();
    const table = screen.getByRole('group', { name: 'Symbol coverage table' });
    expect(within(table).getByText('app.mkactivity')).toBeInTheDocument();
    expect(within(table).getByText('App.MustParse')).toBeInTheDocument();
    expect(within(table).getAllByText('untested').length).toBeGreaterThan(0);
    // The inventory command is shown so the list is auditable, not hand-written.
    expect(screen.getByText(/Object.keys\(require\('express'\)\)/)).toBeInTheDocument();
  });

  it('filters the symbol table down to one status', async () => {
    renderRecord();
    const chips = screen.getAllByRole('button', { name: /^missing/ });
    await userEvent.click(chips[0]);
    const table = screen.getByRole('group', { name: 'Symbol coverage table' });
    expect(within(table).queryByText('app.get')).toBeNull();
    expect(within(table).getByText('app.mkactivity')).toBeInTheDocument();
  });

  it('lists every case with the symbols it exercised, searchable', async () => {
    renderRecord();
    const cases = screen.getByRole('group', { name: 'Case results table' });
    expect(within(cases).getByText('etag-weak')).toBeInTheDocument();
    expect(within(cases).getByText('Go sorts map keys')).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText('Find a case'), 'etag');
    expect(within(cases).getByText('etag-weak')).toBeInTheDocument();
    expect(within(cases).queryByText('get-basic')).toBeNull();
  });

  it('counts a deviation apart from a mismatch', () => {
    renderRecord();
    const tiles = screen.getByText('documented deviations').parentElement as HTMLElement;
    expect(within(tiles).getByText('1')).toBeInTheDocument();
  });

  it('mounts a nested port only when it is opened, and says what it ports', async () => {
    renderRecord();
    expect(screen.getByRole('heading', { level: 3, name: /Nested package ports \(1\)/ })).toBeInTheDocument();
    expect(screen.queryByRole('group', { name: 'Nested package ports' })).not.toBeNull();
    const disc = screen.getByRole('button', { name: /qs/ });
    expect(disc).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByText('parity/express/nested/qs/go/run.go')).toBeNull();
    await userEvent.click(disc);
    expect(disc).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getAllByText('parity/express/nested/qs/go/run.go').length).toBeGreaterThan(0);
    expect(screen.getByText('qs.Stringify')).toBeInTheDocument();
  });

  it('reports a security finding raised by a case', () => {
    renderRecord();
    expect(screen.getByRole('heading', { name: /Security findings/ })).toBeInTheDocument();
    expect(screen.getByText('Weak ETag comparison admits a stale body')).toBeInTheDocument();
  });
});
