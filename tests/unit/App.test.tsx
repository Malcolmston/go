import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Home } from '../../src/components/Home';
import { Faq } from '../../src/components/Faq';
import { About } from '../../src/components/About';
import { LibHero } from '../../src/components/lib/LibHero';
import { LIBS } from '../../src/data';

// The old top-level <App> (hash-routed tabs wrapped in the shared Layout) is
// gone: routing moved to the Next App Router (app/layout.tsx + app/<route>/
// pages), and the nav/Layout behaviour it used to compose is now covered by the
// go-ui Layout test. What remains worth asserting at this level is that each
// route's view renders its expected landmark heading — i.e. that the pages the
// router mounts show the right content. So this suite renders the specific
// views the routes mount (rather than an <App/> that no longer exists) and
// checks those headings.
const express = LIBS.find((l) => l.id === 'express')!;

describe('route views', () => {
  beforeEach(() => {
    // Views that mount VersionBadge/ReleaseList/DocsApp fetch on mount; keep
    // those requests pending so the views render deterministically.
    global.fetch = vi.fn().mockReturnValue(new Promise(() => {}));
  });

  it('renders the Home view with its hero heading', () => {
    render(<Home go={() => {}} />);
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(/reimagined/i);
  });

  it('renders the FAQ view', () => {
    render(<Faq />);
    expect(
      screen.getByRole('heading', { level: 2, name: /Frequently asked questions/ }),
    ).toBeInTheDocument();
  });

  it('renders a library view (its shared hero heading)', () => {
    render(<LibHero lib={express} key={express.id} />);
    expect(screen.getByRole('heading', { level: 2, name: /Express/ })).toBeInTheDocument();
  });

  it('renders the About view', () => {
    render(<About />);
    expect(screen.getByRole('heading', { level: 2, name: 'About' })).toBeInTheDocument();
  });
});
