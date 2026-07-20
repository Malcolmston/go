import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Layout } from 'go-ui';
import type { Tab, Brand } from 'go-ui';

const brand: Brand = { title: 'malcolmston', sub: '/go', initials: 'go', href: '#home' };
const tabs: Tab[] = [
  { id: 'home', label: 'Home' },
  { id: 'express', label: 'Express', dot: '#00add8' },
  { id: 'about', label: 'About' },
];

describe('Layout', () => {
  it('renders brand, tabs, theme toggle and github link', () => {
    render(
      <Layout brand={brand} tabs={tabs} active="home" onNav={() => {}} github="https://github.com/malcolmston" footer={<span>footer text</span>}>
        <div>page body</div>
      </Layout>,
    );
    expect(screen.getByText('malcolmston')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Express' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Toggle colour theme/ })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /GitHub/ })).toHaveAttribute('href', 'https://github.com/malcolmston');
    expect(screen.getByText('page body')).toBeInTheDocument();
    expect(screen.getByText('footer text')).toBeInTheDocument();
  });

  it('marks the active tab and calls onNav when a tab is clicked', async () => {
    const onNav = vi.fn();
    render(
      <Layout brand={brand} tabs={tabs} active="home" onNav={onNav}>
        <div />
      </Layout>,
    );
    expect(screen.getByRole('link', { name: 'Home' }).className).toContain('active');
    await userEvent.click(screen.getByRole('link', { name: 'About' }));
    expect(onNav).toHaveBeenCalledWith('about');
  });

  it('toggles the mobile menu open and closed', async () => {
    render(
      <Layout brand={brand} tabs={tabs} active="home" onNav={() => {}}>
        <div />
      </Layout>,
    );
    const menuBtn = screen.getByRole('button', { name: 'Menu' });
    const navEl = document.querySelector('nav.tabs') as HTMLElement;
    expect(navEl.className).not.toContain('open');
    await userEvent.click(menuBtn);
    expect(navEl.className).toContain('open');
  });
});
