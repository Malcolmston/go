import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Explore } from '../../../src/components/Explore';

// The Explore tab talks to the serverless search + package-graph API when it is
// available, and otherwise degrades to the data files bundled in web/public.
// These tests only exercise that offline/fallback path: global.fetch is stubbed
// so the API health probe fails (no serverless functions, e.g. GitHub Pages)
// while the bundled graph.json / search-index.json resolve to empty payloads.
// We assert the component renders its shell — search box + a status badge —
// and never assert on network-dependent content (hits, nodes, edges).
describe('Explore', () => {
  beforeEach(() => {
    global.fetch = vi.fn((input: RequestInfo | URL) => {
      const url = String(typeof input === 'object' && 'url' in input ? input.url : input);
      // API probe / live endpoints are unavailable in this environment: reject so
      // hasApi() resolves to "no API" and the component uses the bundled fallback.
      if (url.includes('/api/')) {
        return Promise.reject(new Error('no serverless functions'));
      }
      // Bundled fallback data files resolve to well-formed but empty payloads.
      const body = url.includes('search-index')
        ? { generatedAt: '2026-01-01T00:00:00Z', symbols: [] }
        : { generatedAt: '2026-01-01T00:00:00Z', libraries: [], packages: [], edges: [] };
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(body),
        text: () => Promise.resolve(JSON.stringify(body)),
      } as Response);
    });
  });

  it('renders without throwing and shows a search input', () => {
    const { container } = render(<Explore />);
    // A text/search input for querying symbols is always present.
    const input = container.querySelector('input');
    expect(input).not.toBeNull();
  });

  it('shows the fallback badge when the live API is unavailable', async () => {
    render(<Explore />);
    // Once the health probe fails and the bundled data loads, the component
    // reports that it is running on the bundled fallback (not Elasticsearch /
    // GraphQL). Match tolerantly so the exact wording can evolve.
    await waitFor(() => {
      expect(screen.getByText(/fallback|bundled|offline/i)).toBeInTheDocument();
    });
    // Note: hasApi() memoizes its health probe (probe once per session), so we
    // assert on the resulting badge rather than on fetch call counts, which are
    // sensitive to that cache across tests in the same module.
  });

  it('labels the search box for assistive tech', () => {
    render(<Explore />);
    // The box had only a placeholder, which is not an accessible name.
    expect(screen.getByRole('searchbox', { name: /Search symbols/i })).toBeInTheDocument();
  });

  it('announces the result count in a live region', async () => {
    render(<Explore />);
    await userEvent.type(screen.getByRole('searchbox'), 'Router');
    await waitFor(() => {
      const status = screen.getByRole('status');
      expect(status.textContent).toMatch(/Searching…|results?/);
    });
  });

  it('leaves the search-clear button out of the tab order until there is a query', async () => {
    render(<Explore />);
    expect(screen.queryByRole('button', { name: 'Clear search' })).toBeNull();
    await userEvent.type(screen.getByRole('searchbox'), 'Router');
    const clear = screen.getByRole('button', { name: 'Clear search' });
    // Inside no <form>, but an implicit type="submit" is still a footgun.
    expect(clear).toHaveAttribute('type', 'button');
    await userEvent.click(clear);
    expect(screen.getByRole('searchbox')).toHaveValue('');
  });

  it('recovers from a rejected search instead of spinning forever', async () => {
    render(<Explore />);
    await userEvent.type(screen.getByRole('searchbox'), 'Router');
    // /api/* rejects in this suite and the bundled index is empty, so the query
    // must settle on an explicit "no results" rather than staying in "Searching…".
    await waitFor(
      () => {
        expect(screen.getByRole('status').textContent).not.toMatch(/Searching…/);
      },
      { timeout: 3000 },
    );
    expect(screen.getByText(/No symbols match/)).toBeInTheDocument();
  });

  // The library dropdown was removed: the tab now renders ONE graph containing
  // every package of every library, so there is nothing to pick between.
  it('renders a single graph with no library picker', async () => {
    render(<Explore />);
    await waitFor(() => {
      expect(screen.getByText(/no graph data available/)).toBeInTheDocument();
    });
    expect(screen.queryByLabelText('Library graph')).toBeNull();
    expect(screen.queryByRole('combobox')).toBeNull();
    expect(screen.getByText('Package graph')).toBeInTheDocument();
  });

  it('summarises the whole corpus rather than one library', async () => {
    // src/api/graph.ts memoizes the bundled graph in a module-level cache, so
    // the empty payload from the tests above would otherwise be reused here.
    // Reset the registry and re-import so this case gets its own fetch.
    vi.resetModules();
    const pkgs = [
      { id: 'a/1', library: 'alpha', importPath: 'github.com/x/alpha', name: 'alpha', synopsis: '', symbolCount: 3 },
      { id: 'a/2', library: 'alpha', importPath: 'github.com/x/alpha/sub', name: 'sub', synopsis: '', symbolCount: 1 },
      { id: 'b/1', library: 'beta', importPath: 'github.com/x/beta', name: 'beta', synopsis: '', symbolCount: 2 },
    ];
    global.fetch = vi.fn((input: RequestInfo | URL) => {
      const url = String(typeof input === 'object' && 'url' in input ? input.url : input);
      if (url.includes('/api/')) return Promise.reject(new Error('no serverless functions'));
      const body = url.includes('search-index')
        ? { generatedAt: 'x', symbols: [] }
        : {
            generatedAt: 'x',
            libraries: [
              { id: 'alpha', name: 'alpha', packageCount: 2 },
              { id: 'beta', name: 'beta', packageCount: 1 },
            ],
            packages: pkgs,
            edges: [],
          };
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(body),
        text: () => Promise.resolve(JSON.stringify(body)),
      } as Response);
    });

    const { Explore: Fresh } = await import('../../../src/components/Explore');
    const { container } = render(<Fresh />);
    // All three packages from both libraries are drawn in the one graph. The
    // caption is assembled from several JSX children, so match on the
    // collapsed text of the whole pane rather than a single text node.
    await waitFor(() => {
      const text = (container.textContent ?? '').replace(/\s+/g, ' ');
      expect(text).toMatch(/3 packages across 2 libraries/);
    });
  });

  // Selecting a package opens the left-hand info panel, which reports the
  // package's details and its hop distance to the rest of the graph.
  it('opens a dismissable details panel with cluster distance for a selected package', async () => {
    vi.resetModules();
    const pkgs = [
      { id: 'github.com/x/alpha', library: 'alpha', importPath: 'github.com/x/alpha', name: 'alpha', synopsis: 'Root of alpha.', symbolCount: 3 },
      { id: 'github.com/x/alpha/sub', library: 'alpha', importPath: 'github.com/x/alpha/sub', name: 'sub', synopsis: '', symbolCount: 1 },
      { id: 'github.com/x/beta', library: 'beta', importPath: 'github.com/x/beta', name: 'beta', synopsis: '', symbolCount: 2 },
    ];
    const body = {
      generatedAt: 'x',
      libraries: [
        { id: 'alpha', name: 'alpha', packageCount: 2, parityAfter: '97%' },
        { id: 'beta', name: 'beta', packageCount: 1 },
      ],
      packages: pkgs,
      edges: [
        { source: 'github.com/x/alpha/sub', target: 'github.com/x/alpha', kind: 'same-library', weight: 1 },
        { source: 'github.com/x/alpha/sub', target: 'github.com/x/beta', kind: 'import', weight: 4 },
      ],
    };
    global.fetch = vi.fn((input: RequestInfo | URL) => {
      const url = String(typeof input === 'object' && 'url' in input ? input.url : input);
      if (url.includes('/api/')) return Promise.reject(new Error('no serverless functions'));
      const payload = url.includes('search-index') ? { generatedAt: 'x', symbols: [] } : body;
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-length': '10' }),
        json: () => Promise.resolve(payload),
        text: () => Promise.resolve(JSON.stringify(payload)),
      } as Response);
    });

    const { Explore: Fresh } = await import('../../../src/components/Explore');
    render(<Fresh />);
    const node = await waitFor(() => {
      const el = document.querySelector('.xpl-node[title="github.com/x/alpha/sub"]');
      if (!el) throw new Error('node not rendered yet');
      return el as HTMLElement;
    });
    await userEvent.click(node);

    const panel = screen.getByRole('complementary', { name: /github\.com\/x\/alpha\/sub/ });
    const text = (panel.textContent ?? '').replace(/\s+/g, ' ');
    // Identity + measured/declared parity + the distance metric it is showing.
    expect(text).toMatch(/github\.com\/x\/alpha\/sub/);
    expect(text).toMatch(/97%/);
    expect(text).toMatch(/Cluster distance/);
    expect(text).toMatch(/Hop count = shortest path over/);
    // Both neighbours are one hop away over all edge kinds.
    expect(text).toMatch(/Nearest packages/);
    expect(text).toMatch(/2 of 2 packages reachable/);

    // Restricting the metric to dependency edges drops the same-library hop.
    await userEvent.click(screen.getByRole('button', { name: 'dependency edges' }));
    expect((panel.textContent ?? '').replace(/\s+/g, ' ')).toMatch(/1 of 2 packages reachable/);

    // And the panel can be dismissed.
    await userEvent.click(screen.getByRole('button', { name: 'Close package details' }));
    expect(screen.queryByRole('complementary')).toBeNull();
  });

  // The search box highlights matches in place: every node stays on the map.
  it('highlights search matches without filtering the graph', async () => {
    vi.resetModules();
    const body = {
      generatedAt: 'x',
      libraries: [{ id: 'alpha', name: 'alpha', packageCount: 2 }],
      packages: [
        { id: 'github.com/x/alpha', library: 'alpha', importPath: 'github.com/x/alpha', name: 'alpha', synopsis: '', symbolCount: 3 },
        { id: 'github.com/x/alpha/router', library: 'alpha', importPath: 'github.com/x/alpha/router', name: 'router', synopsis: '', symbolCount: 1 },
      ],
      edges: [],
    };
    global.fetch = vi.fn((input: RequestInfo | URL) => {
      const url = String(typeof input === 'object' && 'url' in input ? input.url : input);
      if (url.includes('/api/')) return Promise.reject(new Error('no serverless functions'));
      const payload = url.includes('search-index') ? { generatedAt: 'x', symbols: [] } : body;
      return Promise.resolve({
        ok: true,
        status: 200,
        headers: new Headers({ 'content-length': '10' }),
        json: () => Promise.resolve(payload),
        text: () => Promise.resolve(JSON.stringify(payload)),
      } as Response);
    });

    const { Explore: Fresh } = await import('../../../src/components/Explore');
    render(<Fresh />);
    await waitFor(() => expect(document.querySelectorAll('.xpl-node')).toHaveLength(2));
    await userEvent.type(screen.getByRole('searchbox'), 'router');
    await waitFor(() => {
      expect(document.querySelectorAll('.xpl-node.is-hit')).toHaveLength(1);
    });
    // Nothing was removed — the non-match is only de-emphasised.
    expect(document.querySelectorAll('.xpl-node')).toHaveLength(2);
    expect(document.querySelectorAll('.xpl-node.is-dim')).toHaveLength(1);
  });
});
