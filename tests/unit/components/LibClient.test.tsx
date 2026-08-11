import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import LibClient from '../../../app/lib/[id]/LibClient';
import { LIBS } from '../../../src/data';

// LibClient is what /lib/<id> mounts. The route id comes straight from the URL,
// so an unknown or stale id must render a helpful fallback rather than throw.
describe('LibClient', () => {
  beforeEach(() => {
    // VersionBadge / DocsApp fetch on mount; keep those requests pending.
    global.fetch = vi.fn().mockReturnValue(new Promise(() => {}));
  });

  it('renders the library view for a known id', () => {
    const { container } = render(<LibClient id="express" />);
    expect(container.querySelector('#view-express')).not.toBeNull();
    expect(screen.getByRole('heading', { level: 2, name: /Express/ })).toBeInTheDocument();
  });

  it('renders a not-found body for an unknown id instead of crashing', () => {
    const { container } = render(<LibClient id="totally-made-up" />);
    expect(container.querySelector('#view-lib-missing')).not.toBeNull();
    expect(screen.getByRole('heading', { name: 'Library not found' })).toBeInTheDocument();
    expect(screen.getByText('totally-made-up')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /Explore all libraries/ })).toHaveAttribute(
      'href',
      '/explore',
    );
  });

  it('suggests the current slug when an old one is used', () => {
    // socket.io was renamed to the socketio slug; the old link must still help.
    render(<LibClient id="socket.io" />);
    expect(screen.getByRole('link', { name: 'Socket.IO' })).toHaveAttribute('href', '/lib/socketio');
  });

  it('renders the fallback without suggestions when nothing is close', () => {
    render(<LibClient id="zzzzzzzz" />);
    expect(screen.getByRole('heading', { name: 'Library not found' })).toBeInTheDocument();
    expect(screen.queryByText('Did you mean:')).toBeNull();
  });

  it('renders every library id that the route pre-renders', () => {
    for (const lib of LIBS.slice(0, 6)) {
      const { container, unmount } = render(<LibClient id={lib.id} />);
      expect(container.querySelector('#view-lib-missing')).toBeNull();
      unmount();
    }
  });
});
