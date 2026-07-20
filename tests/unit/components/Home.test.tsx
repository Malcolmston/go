import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Home } from '../../../src/components/Home';
import { LIBS } from '../../../src/data';

describe('Home', () => {
  beforeEach(() => {
    global.fetch = vi.fn().mockReturnValue(new Promise(() => {}));
  });

  it('renders the hero heading and one card per library', () => {
    const { container } = render(<Home go={() => {}} />);
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(/reimagined/i);
    expect(screen.getByRole('heading', { name: 'Pick a library' })).toBeInTheDocument();
    expect(container.querySelectorAll('.card.lib').length).toBe(LIBS.length);
  });

  it('links to the how-to route from the get-started CTA', () => {
    // The CTA is now a Next <Link> (App Router) rather than a go() callback, so
    // assert it points at the /howto route instead of a hash-tab navigation.
    render(<Home go={() => {}} />);
    expect(screen.getByRole('link', { name: /Get started/ })).toHaveAttribute('href', '/howto');
  });

  it('opens a library when its card is clicked', async () => {
    const go = vi.fn();
    const { container } = render(<Home go={go} />);
    await userEvent.click(container.querySelector('.card.lib') as HTMLElement);
    expect(go).toHaveBeenCalledWith(LIBS[0].id);
  });
});
