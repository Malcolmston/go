import { describe, it, expect } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Parity } from '../../../src/components/Parity';
import { parityFixture, summaryFixture } from './parityFixture';

const rows = summaryFixture();
const generatedAt = parityFixture().generatedAt;
const unaudited = [{ id: 'lucene', name: 'Lucene' }];

function renderPage() {
  return render(<Parity generatedAt={generatedAt} rows={rows} unaudited={unaudited} />);
}

function libTable(): HTMLElement {
  return screen.getByRole('group', { name: 'Measured libraries' }).querySelector('table') as HTMLElement;
}

describe('Parity (index)', () => {
  it('states the method and lists the stages of a run on the same page', () => {
    const { container } = renderPage();
    expect(container.querySelector('#view-parity')).not.toBeNull();
    expect(screen.getByRole('heading', { level: 2, name: /How every parity score is calculated/ })).toBeInTheDocument();
    // The pipeline is no longer a separate tab: its stages are documented here.
    expect(screen.getByRole('heading', { name: 'The stages of a run' })).toBeInTheDocument();
    expect(screen.getByText(/Install the pinned upstream oracle/)).toBeInTheDocument();
    expect(screen.getByText(/Write parity.json and COVERAGE.md/)).toBeInTheDocument();
  });

  it('counts nested harnesses into the totals rather than hiding them', () => {
    renderPage();
    // 2 libraries + 1 nested port = 3 harnesses.
    expect(screen.getByText(/harnesses, including 1 nested package ports?/)).toBeInTheDocument();
    // 4 + 3 express/numpy cases + 2 nested = 9.
    expect(screen.getByText('9')).toBeInTheDocument();
  });

  it('links each library to its own parity record, naming the pinned oracle', () => {
    renderPage();
    const body = libTable().querySelector('tbody') as HTMLElement;
    const link = within(body).getByRole('link', { name: 'Express' });
    expect(link).toHaveAttribute('href', '/parity/express');
    expect(within(body).getByText('express@4.19.2')).toBeInTheDocument();
    expect(within(body).getByText('numpy==2.0.1')).toBeInTheDocument();
  });

  it('filters by free text', async () => {
    renderPage();
    await userEvent.type(screen.getByLabelText('Find a library'), 'numpy');
    const body = libTable().querySelector('tbody') as HTMLElement;
    expect(within(body).getByRole('link', { name: 'NumPy' })).toBeInTheDocument();
    expect(within(body).queryByRole('link', { name: 'Express' })).toBeNull();
  });

  it('filters by upstream ecosystem', async () => {
    renderPage();
    await userEvent.selectOptions(screen.getByLabelText('Ecosystem'), 'python');
    const body = libTable().querySelector('tbody') as HTMLElement;
    expect(within(body).queryByRole('link', { name: 'Express' })).toBeNull();
    expect(within(body).getByRole('link', { name: 'NumPy' })).toBeInTheDocument();
  });

  it('reveals nested ports and the different upstream each one ports', async () => {
    renderPage();
    const toggle = screen.getByRole('button', { name: /show 1/ });
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    await userEvent.click(toggle);
    expect(toggle).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByText(/ports qs@6.11.2/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'qs' })).toHaveAttribute(
      'href',
      '/parity/express#express-nested-qs',
    );
  });

  it('accounts for libraries with no harness instead of dropping them', () => {
    renderPage();
    expect(screen.getByText(/1 library has no harness yet/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Lucene' })).toHaveAttribute('href', '/lib/lucene');
  });

  it('says so plainly when the dataset is missing rather than showing zeros', () => {
    render(<Parity generatedAt="" rows={[]} unaudited={[]} />);
    expect(screen.getByText(/No parity dataset is available in this build/)).toBeInTheDocument();
    expect(screen.queryByRole('group', { name: 'Measured libraries' })).toBeNull();
  });
});
