import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
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
});
