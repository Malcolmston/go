import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { About } from '../../../src/components/About';

describe('About', () => {
  it('renders the about heading and the library summary table', () => {
    const { container } = render(<About />);
    expect(container.querySelector('#view-about')).not.toBeNull();
    expect(screen.getByRole('heading', { level: 2, name: 'About' })).toBeInTheDocument();
    expect(screen.getByRole('table')).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Library' })).toBeInTheDocument();
    expect(screen.getByRole('cell', { name: 'Express' })).toBeInTheDocument();
  });
});
