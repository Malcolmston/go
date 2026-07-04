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

  it('navigates to howto when the get-started CTA is clicked', async () => {
    const go = vi.fn();
    render(<Home go={go} />);
    await userEvent.click(screen.getByRole('link', { name: /Get started/ }));
    expect(go).toHaveBeenCalledWith('howto');
  });

  it('opens a library when its card is clicked', async () => {
    const go = vi.fn();
    const { container } = render(<Home go={go} />);
    await userEvent.click(container.querySelector('.card.lib') as HTMLElement);
    expect(go).toHaveBeenCalledWith(LIBS[0].id);
  });
});
