import { render } from '@testing-library/react';
import { createRef } from 'react';
import { describe, it, expect, vi } from 'vitest';
import { Wormhole } from 'go-ui';
import type { WormholeHandle } from 'go-ui';

describe('Wormhole', () => {
  it('renders a pointer-transparent, aria-hidden canvas overlay', () => {
    const { container } = render(<Wormhole />);
    const canvas = container.querySelector('canvas.wormhole');
    expect(canvas).toBeTruthy();
    expect(canvas?.getAttribute('aria-hidden')).toBe('true');
  });

  it('warp() invokes the arrival callback (routes even when no 2d context is available)', () => {
    const ref = createRef<WormholeHandle>();
    render(<Wormhole ref={ref} />);
    const onArrive = vi.fn();
    // In jsdom canvas.getContext returns null, so warp() falls through to an
    // immediate route — the page must still change.
    ref.current!.warp(onArrive);
    expect(onArrive).toHaveBeenCalledTimes(1);
  });
});
