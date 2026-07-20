import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { AI } from '../../../src/components/AI';

describe('AI', () => {
  it('renders the AI story sections', () => {
    const { container } = render(<AI />);
    expect(container.querySelector('#view-ai')).not.toBeNull();
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(/AI story/);
    expect(screen.getByRole('heading', { name: 'How it was built' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Faithful behaviour' })).toBeInTheDocument();
  });
});
