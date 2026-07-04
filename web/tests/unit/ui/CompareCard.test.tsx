import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { CompareCard } from 'go-ui';

describe('CompareCard', () => {
  it('renders the language name and highlighted code', () => {
    render(<CompareCard name="Node.js" color="#8cc84b" html={'<span>const app</span>'} />);
    expect(screen.getByText('Node.js')).toBeInTheDocument();
    expect(screen.getByText('const app')).toBeInTheDocument();
  });

  it('copies the code to the clipboard on click', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    render(<CompareCard name="Go" color="#2f9bff" html={'app := new()'} />);
    await userEvent.click(screen.getByRole('button', { name: 'Copy' }));
    expect(writeText).toHaveBeenCalled();
    expect(await screen.findByRole('button', { name: 'Copied' })).toBeInTheDocument();
  });
});
