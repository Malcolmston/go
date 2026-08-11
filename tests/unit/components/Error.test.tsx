import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { renderToStaticMarkup } from 'react-dom/server';
import ErrorBoundaryPage from '../../../app/error';
import GlobalError from '../../../app/global-error';

// app/error.tsx renders INSIDE the shared shell (like app/not-found.tsx), so it
// must not introduce a second <main> landmark. app/global-error.tsx REPLACES the
// root layout, so it owns <html>/<body> and may not depend on the app stylesheet
// — it is checked as server markup because <html> cannot be mounted in jsdom.
describe('segment error boundary', () => {
  it('renders a heading, a retry button and recovery links', () => {
    render(<ErrorBoundaryPage error={new Error('boom')} reset={() => {}} />);
    expect(screen.getByRole('heading', { name: 'Something went wrong' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Try again' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /Home/ })).toHaveAttribute('href', '/');
    expect(screen.getByRole('link', { name: /Explore/ })).toHaveAttribute('href', '/explore');
  });

  it('calls reset when the retry button is clicked', () => {
    const reset = vi.fn();
    render(<ErrorBoundaryPage error={new Error('boom')} reset={reset} />);
    screen.getByRole('button', { name: 'Try again' }).click();
    expect(reset).toHaveBeenCalledTimes(1);
  });

  it('shows the digest but never the message', () => {
    const err = Object.assign(new Error('secret internal detail'), { digest: '4263957425' });
    render(<ErrorBoundaryPage error={err} reset={() => {}} />);
    expect(screen.getByText('4263957425')).toBeInTheDocument();
    expect(screen.queryByText(/secret internal detail/)).toBeNull();
  });

  it('omits the digest paragraph when there is no digest', () => {
    render(<ErrorBoundaryPage error={new Error('boom')} reset={() => {}} />);
    expect(screen.queryByText(/Reference for a bug report/)).toBeNull();
  });

  it('does not nest a second main landmark inside the shell', () => {
    const { container } = render(<ErrorBoundaryPage error={new Error('boom')} reset={() => {}} />);
    expect(container.querySelector('main')).toBeNull();
    expect(container.querySelector('#view-error')).not.toBeNull();
  });
});

describe('global error boundary', () => {
  it('renders its own document with branding, retry and a home link', () => {
    const err = Object.assign(new Error('secret internal detail'), { digest: 'abc123' });
    const html = renderToStaticMarkup(<GlobalError error={err} reset={() => {}} />);
    expect(html).toContain('<html lang="en">');
    expect(html).toContain('<body');
    expect(html).toContain('malcolmston/go');
    expect(html).toContain('Something went wrong');
    expect(html).toContain('Try again');
    expect(html).toContain('abc123');
    expect(html).toContain('href="/"');
    expect(html).not.toContain('secret internal detail');
  });

  it('styles itself inline so it survives a missing app stylesheet', () => {
    const html = renderToStaticMarkup(<GlobalError error={new Error('boom')} reset={() => {}} />);
    expect(html).toMatch(/<body style="[^"]*background:#08080a/);
    expect(html).not.toContain('<link');
    expect(html).not.toContain('var(--');
  });
});
