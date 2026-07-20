import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { LibCard } from '../../../src/components/LibCard';
import { LIBS } from '../../../src/data';

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
});
