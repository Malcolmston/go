import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { LibCard } from '../../../src/components/LibCard';
import { LIBS } from '../../../src/data';
import { parityFor } from '../../../src/parityLookup';

const express = LIBS.find((l) => l.id === 'express')!;

describe('LibCard', () => {
  beforeEach(() => {
    // VersionBadge fetches on mount; keep it pending so the card renders cleanly.
    global.fetch = vi.fn().mockReturnValue(new Promise(() => {}));
  });

  it('renders the library name, package and tags', () => {
    render(<LibCard lib={express} onOpen={() => {}} />);
    expect(screen.getByRole('heading', { name: /Express/ })).toBeInTheDocument();
    expect(screen.getByText(express.pkg)).toBeInTheDocument();
    expect(screen.getByText(express.tagline)).toBeInTheDocument();
    expect(screen.getByText(express.tags[0])).toBeInTheDocument();
  });

  it('calls onOpen with the library id when clicked', async () => {
    const onOpen = vi.fn();
    const { container } = render(<LibCard lib={express} onOpen={onOpen} />);
    await userEvent.click(container.querySelector('.card.lib') as HTMLElement);
    expect(onOpen).toHaveBeenCalledWith('express');
  });

  it('is operable from the keyboard, not only by mouse', async () => {
    const onOpen = vi.fn();
    render(<LibCard lib={express} onOpen={onOpen} />);
    const card = screen.getByRole('button', { name: new RegExp(express.name) });

    // The whole card is the click target, so it must also be focusable and
    // respond to Enter and Space like any button.
    await userEvent.tab();
    expect(card).toHaveFocus();

    await userEvent.keyboard('{Enter}');
    expect(onOpen).toHaveBeenCalledWith('express');

    onOpen.mockClear();
    await userEvent.keyboard(' ');
    expect(onOpen).toHaveBeenCalledWith('express');
  });

  it('names the card for assistive tech', () => {
    render(<LibCard lib={express} onOpen={() => {}} />);
    expect(screen.getByRole('button')).toHaveAccessibleName(
      `${express.name} — ${express.tagline}`,
    );
  });

  it('shows the measured parity score as text, not colour alone', () => {
    render(<LibCard lib={express} onOpen={() => {}} />);
    const parity = parityFor(express);
    expect(parity).toBeDefined();
    expect(screen.getByText(new RegExp(`${parity!.after} upstream parity`))).toBeInTheDocument();
  });

  it('omits the parity badge for a library with no measured score', () => {
    const opencv = LIBS.find((l) => l.id === 'opencv')!;
    expect(parityFor(opencv)).toBeUndefined();
    render(<LibCard lib={opencv} onOpen={() => {}} />);
    expect(screen.queryByText(/upstream parity/)).toBeNull();
  });
});
