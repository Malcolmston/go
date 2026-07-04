import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Html } from '../../../src/components/Html';

describe('Html', () => {
  it('renders raw markup into a default span', () => {
    const { container } = render(<Html html={'<b>bold</b>'} />);
    expect(screen.getByText('bold').tagName).toBe('B');
    expect(container.querySelector('span')).not.toBeNull();
  });

  it('renders into a custom tag and forwards extra props', () => {
    const { container } = render(<Html tag="li" className="feat" html={'<code>x</code>'} />);
    const li = container.querySelector('li');
    expect(li).not.toBeNull();
    expect(li).toHaveClass('feat');
    expect(screen.getByText('x').tagName).toBe('CODE');
  });
});
