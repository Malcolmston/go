import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { LibView } from '../../../src/components/LibView';
import { LIBS } from '../../../src/data';
import { parityFor } from '../../../src/parityLookup';

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

  it('exposes the sub-sections as an ARIA tablist wired to its panels', async () => {
    render(<LibView lib={express} />);
    const tablist = screen.getByRole('tablist', { name: /Express sections/ });
    const tabs = within(tablist).getAllByRole('tab');
    expect(tabs.map((t) => t.textContent)).toEqual(['Overview', 'Examples', 'API', 'Parity']);

    // Exactly one tab is selected and it is the only one in the tab order.
    expect(tabs.filter((t) => t.getAttribute('aria-selected') === 'true')).toHaveLength(1);
    expect(tabs.filter((t) => t.getAttribute('tabindex') === '0')).toHaveLength(1);

    // The selected tab points at the visible panel, which points back at it.
    const panel = screen.getByRole('tabpanel');
    expect(tabs[0]).toHaveAttribute('aria-controls', panel.id);
    expect(panel).toHaveAttribute('aria-labelledby', tabs[0].id);
  });

  it('moves between sub-tabs with the arrow keys', async () => {
    render(<LibView lib={express} />);
    const tabs = within(screen.getByRole('tablist')).getAllByRole('tab');
    tabs[0].focus();

    await userEvent.keyboard('{ArrowRight}');
    expect(tabs[1]).toHaveAttribute('aria-selected', 'true');
    expect(tabs[1]).toHaveFocus();
    expect(screen.getByRole('heading', { name: /→ Go/ })).toBeInTheDocument();

    await userEvent.keyboard('{End}');
    expect(tabs[3]).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('heading', { name: 'Upstream parity' })).toBeInTheDocument();

    await userEvent.keyboard('{Home}');
    expect(tabs[0]).toHaveAttribute('aria-selected', 'true');

    // Wraps backwards from the first tab to the last.
    await userEvent.keyboard('{ArrowLeft}');
    expect(tabs[3]).toHaveAttribute('aria-selected', 'true');
  });

  it('shows the measured parity numbers on the Parity sub-tab', async () => {
    render(<LibView lib={express} />);
    await userEvent.click(screen.getByRole('tab', { name: 'Parity' }));
    const parity = parityFor(express);
    expect(parity).toBeDefined();
    expect(screen.getAllByText(parity!.after).length).toBeGreaterThan(0);
    expect(screen.getByText('cases asked of both')).toBeInTheDocument();
  });

  it('says so plainly when a library has no upstream parity score', async () => {
    // opencv has no parity.json entry, so its Parity tab must not imply one.
    const opencv = LIBS.find((l) => l.id === 'opencv')!;
    expect(parityFor(opencv)).toBeUndefined();
    render(<LibView lib={opencv} />);
    await userEvent.click(screen.getByRole('tab', { name: 'Parity' }));
    expect(screen.getByText(/not yet audited against an upstream test suite/)).toBeInTheDocument();
    expect(screen.queryByText('measured parity')).toBeNull();
  });

  it('jumps to the API reference from the hero "API docs" pill', async () => {
    render(<LibView lib={express} />);
    await userEvent.click(screen.getByRole('button', { name: /API docs/ }));
    expect(screen.getByRole('tab', { name: 'API' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('heading', { name: 'API reference' })).toBeInTheDocument();
  });
});
