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
    const github = screen.getByRole('link', { name: /GitHub/ });
    expect(github).toHaveAttribute('href', express.repo);
    expect(github).toHaveAttribute('target', '_blank');
    expect(github).toHaveAttribute('rel', expect.stringContaining('noopener'));
  });
});
