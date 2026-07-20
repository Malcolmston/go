import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { CodeBlock } from 'go-ui';

describe('CodeBlock', () => {
  it('renders highlighted html and the language label', () => {
    render(<CodeBlock lang="main.go" html={'<span class="tok-k">func</span> main'} />);
    expect(screen.getByText('main.go')).toBeInTheDocument();
    expect(screen.getByText('func')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Copy' })).toBeInTheDocument();
  });

  it('renders plain text when no html is provided', () => {
    render(<CodeBlock lang="shell" text="go build ./..." />);
    expect(screen.getByText('go build ./...')).toBeInTheDocument();
  });

  it('copies to clipboard and flips the button label', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    render(<CodeBlock lang="go" text="hello" />);
    await userEvent.click(screen.getByRole('button', { name: 'Copy' }));
    expect(writeText).toHaveBeenCalled();
    expect(await screen.findByRole('button', { name: 'Copied' })).toBeInTheDocument();
  });
});
