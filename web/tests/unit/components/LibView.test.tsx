import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { LibView } from '../../../src/components/LibView';
import { LIBS } from '../../../src/data';

const express = LIBS.find((l) => l.id === 'express')!;

describe('LibView', () => {
  beforeEach(() => {
    global.fetch = vi.fn().mockReturnValue(new Promise(() => {}));
  });

  it('renders the library hero, install command and feature list', () => {
    const { container } = render(<LibView lib={express} />);
    expect(container.querySelector('#view-express')).not.toBeNull();
    expect(screen.getByRole('heading', { level: 2, name: /Express/ })).toBeInTheDocument();
    expect(screen.getByText(express.tagline)).toBeInTheDocument();
    expect(screen.getByText(new RegExp(`go get ${express.pkg}`))).toBeInTheDocument();
    // "Features" section heading and at least one feature bullet.
    expect(screen.getByRole('heading', { name: 'Features' })).toBeInTheDocument();
    expect(container.querySelectorAll('ul.feat li').length).toBe(express.features.length);
  });

  it('renders external doc/repo links opening in a new tab', () => {
    render(<LibView lib={express} />);
    // Several GitHub links now render (the hero repo link plus the parity
    // pipeline's run/actions link); every external GitHub link must open safely
    // in a new tab, and the hero one points at the repo.
    const githubs = screen.getAllByRole('link', { name: /GitHub/ });
    expect(githubs.length).toBeGreaterThan(0);
    for (const link of githubs) {
      expect(link).toHaveAttribute('target', '_blank');
      expect(link).toHaveAttribute('rel', expect.stringContaining('noopener'));
    }
    expect(githubs.some((l) => l.getAttribute('href') === express.repo)).toBe(true);
  });
});
