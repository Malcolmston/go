import { describe, it, expect } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import ParityLibLoading from '../../../app/parity/[lib]/loading';
import SecurityLoading from '../../../app/security/loading';

// The pending UI for the two heavy App Router segments. Like app/error.tsx and
// app/not-found.tsx these render INSIDE the shared shell, so neither may
// introduce a second <main> landmark, and each must use the same
// `.view active` section wrapper and opening heading level as the real view so
// the frame and the heading order do not jump when the payload lands.
//
// The point of these files is that a slow navigation is both DRAWN and
// ANNOUNCED: hence role="status" + aria-busy on the live text, and
// aria-hidden on the purely decorative skeleton bars.
//
// There is deliberately NO app/loading.tsx: a root boundary would put a
// fallback in front of the server-rendered content of all 92 prerendered
// pages, including the ones whose payload is a few kilobytes.
describe.each([
  ['parity library', ParityLibLoading, /Loading the parity record/],
  ['security', SecurityLoading, /Loading the security findings/],
])('%s loading boundary', (_name, Loading, heading) => {
  it('renders the same section wrapper and heading level as the real view', () => {
    const { container } = render(<Loading />);
    expect(screen.getByRole('heading', { level: 2, name: heading })).toBeInTheDocument();
    expect(container.querySelector('section.view.active')).not.toBeNull();
    expect(container.querySelector('main')).toBeNull();
  });

  it('announces the pending state through a live region', () => {
    render(<Loading />);
    const status = screen.getByRole('status');
    expect(status).toHaveAttribute('aria-busy', 'true');
    expect(status.textContent).toBeTruthy();
  });

  it('draws skeleton bars and hides them from assistive technology', () => {
    const { container } = render(<Loading />);
    const skeleton = container.querySelector('.sk-rows');
    expect(skeleton).not.toBeNull();
    expect(skeleton).toHaveAttribute('aria-hidden', 'true');
    expect(within(skeleton as HTMLElement).queryAllByRole('status')).toHaveLength(0);
    expect(skeleton!.querySelectorAll('.sk-bar').length).toBeGreaterThan(3);
  });
});
