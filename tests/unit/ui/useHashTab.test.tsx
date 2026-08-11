import { describe, it, expect, afterEach } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useHashTab } from 'go-ui';

function Harness({ ids, fallback }: { ids: string[]; fallback: string }) {
  const [active, go] = useHashTab(ids, fallback);
  return (
    <div>
      <span data-testid="active">{active}</span>
      {ids.map((id) => (
        <button key={id} onClick={() => go(id)}>
          go {id}
        </button>
      ))}
    </div>
  );
}

const IDS = ['home', 'faq', 'about'];

// jsdom dispatches hashchange asynchronously; wrap the wait in act so React
// processes the resulting state update.
async function setHash(h: string) {
  await act(async () => {
    window.location.hash = h;
    await new Promise((r) => setTimeout(r, 0));
  });
}

afterEach(() => {
  window.location.hash = '';
});

describe('useHashTab', () => {
  it('falls back when the hash is empty', () => {
    render(<Harness ids={IDS} fallback="home" />);
    expect(screen.getByTestId('active')).toHaveTextContent('home');
  });

  it('picks up a deep-linked tab from the initial hash', async () => {
    await setHash('#faq');
    render(<Harness ids={IDS} fallback="home" />);
    expect(screen.getByTestId('active')).toHaveTextContent('faq');
  });

  it('follows external hash changes (back/forward navigation)', async () => {
    render(<Harness ids={IDS} fallback="home" />);
    await setHash('#about');
    expect(screen.getByTestId('active')).toHaveTextContent('about');
    await setHash('#faq');
    expect(screen.getByTestId('active')).toHaveTextContent('faq');
  });

  it('returns to the fallback when Back clears the hash', async () => {
    // Regression: an empty hash used to be treated as "not a tab id", so
    // pressing Back to the entry URL left the previous tab selected.
    render(<Harness ids={IDS} fallback="home" />);
    await setHash('#about');
    expect(screen.getByTestId('active')).toHaveTextContent('about');
    await setHash('');
    expect(screen.getByTestId('active')).toHaveTextContent('home');
  });

  it('ignores an unknown hash so in-page anchors do not reset the tab', async () => {
    render(<Harness ids={IDS} fallback="home" />);
    await setHash('#faq');
    await setHash('#faq-install');
    expect(screen.getByTestId('active')).toHaveTextContent('faq');
  });

  it('ignores a nested route hash owned by another component', async () => {
    render(<Harness ids={IDS} fallback="home" />);
    await setHash('#about');
    await setHash('#pkg/github.com/x/y');
    expect(screen.getByTestId('active')).toHaveTextContent('about');
  });

  it('updates the hash when go() is called', async () => {
    render(<Harness ids={IDS} fallback="home" />);
    await act(async () => {
      await userEvent.click(screen.getByRole('button', { name: 'go faq' }));
      await new Promise((r) => setTimeout(r, 0));
    });
    expect(window.location.hash).toBe('#faq');
    expect(screen.getByTestId('active')).toHaveTextContent('faq');
  });

  it('re-selects the current tab when go() targets the active hash', async () => {
    await setHash('#faq');
    render(<Harness ids={IDS} fallback="home" />);
    await act(async () => {
      await userEvent.click(screen.getByRole('button', { name: 'go faq' }));
    });
    expect(screen.getByTestId('active')).toHaveTextContent('faq');
    expect(window.location.hash).toBe('#faq');
  });

  it('recognises tabs added after mount (no stale id list)', async () => {
    const { rerender } = render(<Harness ids={['home']} fallback="home" />);
    rerender(<Harness ids={['home', 'later']} fallback="home" />);
    await setHash('#later');
    expect(screen.getByTestId('active')).toHaveTextContent('later');
  });
});
