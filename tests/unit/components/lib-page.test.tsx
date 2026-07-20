import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { LibHero } from '../../../src/components/lib/LibHero';
import { LibNav } from '../../../src/components/lib/LibNav';
import { OverviewPanel } from '../../../src/components/lib/OverviewPanel';
import { LIBS } from '../../../src/data';

const express = LIBS.find((l) => l.id === 'express')!;

describe('library page (split into individual sub-pages)', () => {
  beforeEach(() => {
    // VersionBadge fetches the latest release; keep it pending so nothing renders.
    global.fetch = vi.fn().mockReturnValue(new Promise(() => {}));
  });

  it('LibHero renders the name, tagline and a new-tab GitHub link to the repo', () => {
    render(<LibHero lib={express} />);
    expect(screen.getByRole('heading', { level: 2, name: /Express/ })).toBeInTheDocument();
    expect(screen.getByText(express.tagline)).toBeInTheDocument();
    const gh = screen.getByRole('link', { name: /GitHub/ });
    expect(gh).toHaveAttribute('href', express.repo);
    expect(gh).toHaveAttribute('target', '_blank');
    expect(gh).toHaveAttribute('rel', expect.stringContaining('noopener'));
    // The "API docs" action links to the library's API sub-page (a real route).
    expect(screen.getByRole('link', { name: /API docs/ })).toHaveAttribute('href', `/lib/${express.id}/api`);
  });

  it('OverviewPanel renders the install command and the full feature list', () => {
    const { container } = render(<OverviewPanel lib={express} />);
    expect(screen.getByText(new RegExp(`go get ${express.pkg}`))).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Features' })).toBeInTheDocument();
    expect(container.querySelectorAll('ul.feat li').length).toBe(express.features.length);
  });

  it('LibNav renders the four sub-pages as real routed links', () => {
    render(<LibNav libId={express.id} libName={express.name} accent={express.accent} />);
    const base = `/lib/${express.id}`;
    const expected: [RegExp, string][] = [
      [/Overview/, base],
      [/Examples/, `${base}/examples`],
      [/API/, `${base}/api`],
      [/Parity/, `${base}/parity`],
    ];
    for (const [name, href] of expected) {
      expect(screen.getByRole('tab', { name }).getAttribute('href')).toBe(href);
    }
  });
});
