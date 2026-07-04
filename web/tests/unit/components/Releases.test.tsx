import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Releases } from '../../../src/components/Releases';
import { LIBS } from '../../../src/data';

describe('Releases', () => {
  beforeEach(() => {
    global.fetch = vi.fn().mockResolvedValue({ ok: true, status: 200, json: () => Promise.resolve([]) });
  });

  it('renders the heading and a release block for every library plus the meta repo', () => {
    const { container } = render(<Releases />);
    expect(screen.getByRole('heading', { level: 2, name: /Releases/ })).toBeInTheDocument();
    // One LibReleases block per library, plus the aggregate malcolmston/go entry.
    expect(container.querySelectorAll('.rel-lib').length).toBe(LIBS.length + 1);
    expect(screen.getByRole('heading', { name: 'malcolmston/go' })).toBeInTheDocument();
  });
});
