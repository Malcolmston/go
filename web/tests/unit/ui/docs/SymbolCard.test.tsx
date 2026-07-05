import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { SymbolCard } from 'go-ui';

describe('SymbolCard', () => {
  it('renders an anchored heading, highlighted signature and doc', () => {
    const { container } = render(
      <SymbolCard
        id="sym-New"
        heading="func New"
        signature="func New() *Router"
        doc="New allocates and returns an empty Router."
      />,
    );
    // anchor id for deep linking
    expect(container.querySelector('#sym-New')).not.toBeNull();
    expect(screen.getByRole('heading', { name: /func New/ })).toBeInTheDocument();
    // deep-link anchor points at its own id
    expect(screen.getByRole('link', { name: /Link to func New/ })).toHaveAttribute('href', '#sym-New');
    // signature is present (highlighter tokenizes "func")
    expect(container.textContent).toContain('func New() *Router');
    expect(screen.getByText(/New allocates and returns/)).toBeInTheDocument();
  });

  it('renders nested member children', () => {
    render(
      <SymbolCard id="sym-Router" heading="type Router" signature="type Router struct{}">
        <div data-testid="member">child</div>
      </SymbolCard>,
    );
    expect(screen.getByTestId('member')).toBeInTheDocument();
  });

  it('omits the doc block when no doc is given', () => {
    const { container } = render(<SymbolCard id="sym-x" heading="var X" signature="var X int" />);
    expect(container.querySelector('.doc-text')).toBeNull();
  });
});
