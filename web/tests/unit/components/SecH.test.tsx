import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { SecH } from '../../../src/components/SecH';

describe('SecH', () => {
  it('renders children in a default h3 heading', () => {
    render(<SecH>Pick a library</SecH>);
    const heading = screen.getByRole('heading', { name: 'Pick a library' });
    expect(heading.tagName).toBe('H3');
  });

  it('honours the h override', () => {
    render(<SecH h="h2">About</SecH>);
    expect(screen.getByRole('heading', { name: 'About' }).tagName).toBe('H2');
  });
});
