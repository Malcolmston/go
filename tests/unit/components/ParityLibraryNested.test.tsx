// The nested harnesses are NOT part of the page payload: /parity/<lib> ships the
// collapsed-row summaries only, and the bodies are fetched from
// /parity/<lib>/nested the first time a disclosure is opened. These tests drive
// the component the way the page does — a harness with an empty `nested` map plus
// an explicit summary list — and assert nothing is requested until an open.
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ParityLibrary } from '../../../src/components/ParityLibrary';
import { nestedSummaries } from '../../../src/components/ParityNested';
import { harnessFixture, parityFixture } from './parityFixture';

const full = harnessFixture('express');
const { nested, ...rest } = full;
const summaries = nestedSummaries(nested);

function renderSplit() {
  return render(
    <ParityLibrary
      slug="express"
      name="Express"
      accent="#38bdf8"
      repo="https://github.com/malcolmston/express"
      libRoute="/lib/express"
      generatedAt={parityFixture().generatedAt}
      harness={{ ...rest, nested: {} }}
      nested={summaries}
    />,
  );
}

function mockFetch(body: unknown, init: { ok?: boolean; bytes?: number } = {}) {
  const text = JSON.stringify(body);
  // Typed with the one argument the component passes, so `mock.calls[0][0]`
  // below is a string rather than an index into an empty tuple.
  const fn = vi.fn(async (_url: string) => ({
    ok: init.ok ?? true,
    headers: new Headers({ 'content-length': String(init.bytes ?? text.length) }),
    body: { cancel: async () => undefined },
    json: async () => JSON.parse(text),
  }));
  vi.stubGlobal('fetch', fn);
  return fn;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('ParityLibrary nested payload split', () => {
  it('renders the collapsed rows from summaries alone, fetching nothing', () => {
    const fetchMock = mockFetch({ nested });
    renderSplit();
    expect(screen.getByRole('heading', { level: 3, name: /Nested package ports \(1\)/ })).toBeInTheDocument();
    const disc = screen.getByRole('button', { name: /qs/ });
    expect(disc).toHaveAttribute('aria-expanded', 'false');
    // No case or symbol detail of the nested harness is in the document.
    expect(screen.queryByText('parity/express/nested/qs/go/run.go')).toBeNull();
    expect(screen.queryByText('qs.Stringify')).toBeNull();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('fetches this library\'s nested harnesses on the first open and renders them', async () => {
    const fetchMock = mockFetch({ generatedAt: parityFixture().generatedAt, library: 'express', nested });
    renderSplit();
    await userEvent.click(screen.getByRole('button', { name: /qs/ }));
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe('/parity/express/nested');
    expect(await screen.findByText('qs.Stringify')).toBeInTheDocument();
    expect(screen.getAllByText('parity/express/nested/qs/go/run.go').length).toBeGreaterThan(0);
  });

  it('rejects an oversized answer and offers a retry instead of rendering it', async () => {
    const fetchMock = mockFetch({ nested }, { bytes: 64 * 1024 * 1024 });
    renderSplit();
    await userEvent.click(screen.getByRole('button', { name: /qs/ }));
    expect(await screen.findByText(/could not be loaded/)).toBeInTheDocument();
    expect(screen.queryByText('qs.Stringify')).toBeNull();
    // Re-opening retries rather than staying broken.
    await userEvent.click(screen.getByRole('button', { name: /qs/ }));
    await userEvent.click(screen.getByRole('button', { name: /qs/ }));
    expect(fetchMock.mock.calls.length).toBeGreaterThan(1);
  });

  it('opens without a fetch when the caller already holds the nested harnesses', async () => {
    const fetchMock = mockFetch({ nested });
    render(
      <ParityLibrary
        slug="express"
        name="Express"
        accent="#38bdf8"
        repo={null}
        libRoute={null}
        generatedAt={parityFixture().generatedAt}
        harness={full}
      />,
    );
    await userEvent.click(screen.getByRole('button', { name: /qs/ }));
    expect(screen.getAllByText('parity/express/nested/qs/go/run.go').length).toBeGreaterThan(0);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
