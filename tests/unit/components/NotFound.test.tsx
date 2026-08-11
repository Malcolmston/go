import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import NotFound from '../../../app/not-found';

// app/not-found.tsx renders INSIDE the shared shell, which already owns the
// <main> landmark — so the 404 body must not introduce a second one.
describe('not-found page', () => {
  it('renders a heading and recovery links', () => {
    render(<NotFound />);
    expect(screen.getByRole('heading', { name: 'Page not found' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /Home/ })).toHaveAttribute('href', '/');
    expect(screen.getByRole('link', { name: /Explore/ })).toHaveAttribute('href', '/explore');
    expect(screen.getByRole('link', { name: /How-to/ })).toHaveAttribute('href', '/howto');
  });

  it('does not nest a second main landmark inside the shell', () => {
    const { container } = render(<NotFound />);
    expect(container.querySelector('main')).toBeNull();
    expect(container.querySelector('#view-not-found')).not.toBeNull();
  });
});
