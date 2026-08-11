import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { CodeBlock, hi } from 'go-ui';

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

  it('reports failure when both the Clipboard API and the fallback fail', async () => {
    Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true });
    Object.defineProperty(document, 'execCommand', {
      value: () => {
        throw new Error('unsupported');
      },
      configurable: true,
    });
    render(<CodeBlock lang="go" text="hello" />);
    await userEvent.click(screen.getByRole('button', { name: 'Copy' }));
    expect(await screen.findByRole('button', { name: 'Copy failed' })).toBeInTheDocument();
    Object.defineProperty(document, 'execCommand', { value: undefined, configurable: true });
  });

  it('gives the copy button an explicit type so it never submits a form', () => {
    render(<CodeBlock lang="go" text="hello" />);
    expect(screen.getByRole('button', { name: 'Copy' })).toHaveAttribute('type', 'button');
  });

  it('escapes highlighted source rather than injecting markup', () => {
    const { container } = render(<CodeBlock lang="go" html={hi('a := "<img src=x>"')} />);
    expect(container.querySelector('img')).toBeNull();
    expect(container.querySelector('code')?.textContent).toBe('a := "<img src=x>"');
  });
});
