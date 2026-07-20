import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { HowTo } from '../../../src/components/HowTo';

describe('HowTo', () => {
  it('renders the getting-started sections', () => {
    const { container } = render(<HowTo />);
    expect(container.querySelector('#view-howto')).not.toBeNull();
    expect(screen.getByRole('heading', { level: 2, name: 'How to use this' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: /Install/ })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: /A minimal web server/ })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: /Math & data/ })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: /Where to go next/ })).toBeInTheDocument();
  });
});
