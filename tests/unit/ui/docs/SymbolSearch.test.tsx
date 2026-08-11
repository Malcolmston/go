import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
// SymbolSearch is not re-exported from go-ui's index, so import the source
// directly (same relative-source pattern the app uses).
import { SymbolSearch } from '../../../../ui/src/components/docs/SymbolSearch';
import type { DocPackage } from '../../../../ui/src/docs/types';

// jsdom does not implement scrollIntoView; the component calls it to keep the
// active row visible. Stub it so rendering the dropdown doesn't throw.
if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => {};
}

// A tiny two-package fixture: a func `ReadFile`, a type `LRU` with a `Get`
// method (anchor sym-LRU.Get, tests regex-metachar highlighting of "LRU.Get").
const packages: DocPackage[] = [
  {
    importPath: 'github.com/x/io',
    name: 'io',
    synopsis: '',
    doc: '',
    consts: [],
    vars: [],
    types: [
      {
        name: 'LRU',
        signature: 'type LRU struct{}',
        doc: '',
        consts: [],
        vars: [],
        funcs: [],
        methods: [{ name: 'Get', recv: '*LRU', signature: 'func (*LRU) Get()', doc: '' }],
      },
    ],
    funcs: [{ name: 'ReadFile', signature: 'func ReadFile() error', doc: '' }],
  },
];

function typeQuery(text: string) {
  const input = screen.getByRole('combobox');
  fireEvent.focus(input);
  fireEvent.change(input, { target: { value: text } });
  return input;
}

describe('SymbolSearch (local fallback, no endpoint)', () => {
  it('highlights local query tokens in matched results', () => {
    render(<SymbolSearch packages={packages} onPick={() => {}} />);
    typeQuery('read');
    const marks = document.querySelectorAll('mark.gd-search-mark');
    expect(marks.length).toBeGreaterThan(0);
    expect(Array.from(marks).some((m) => /read/i.test(m.textContent ?? ''))).toBe(true);
  });

  it('handles a query with regex metacharacters without crashing', () => {
    render(<SymbolSearch packages={packages} onPick={() => {}} />);
    typeQuery('LRU.Get');
    // The LRU.Get method row should be present and highlighted (escapeRegExp).
    expect(document.querySelector('.gd-search-item')).toBeTruthy();
    expect(document.querySelectorAll('mark.gd-search-mark').length).toBeGreaterThan(0);
  });

  it('shows the empty state when nothing matches', () => {
    render(<SymbolSearch packages={packages} onPick={() => {}} />);
    typeQuery('zzzznotathing');
    expect(document.querySelector('.gd-search-empty')).toBeTruthy();
  });

  it('ranks a name prefix above a package-only match', () => {
    render(<SymbolSearch packages={packages} onPick={() => {}} />);
    typeQuery('lru');
    const first = document.querySelector('.gd-search-item .gd-search-item-label');
    expect(first?.textContent).toBe('LRU');
  });

  it('arrow keys move the active row and expose it via aria-activedescendant', () => {
    render(<SymbolSearch packages={packages} onPick={() => {}} />);
    const input = typeQuery('lru');
    expect(document.querySelectorAll('.gd-search-item').length).toBeGreaterThan(1);
    expect(input).toHaveAttribute('aria-activedescendant', 'gd-search-opt-0');

    fireEvent.keyDown(input, { key: 'ArrowDown' });
    expect(input).toHaveAttribute('aria-activedescendant', 'gd-search-opt-1');
    expect(document.querySelectorAll('.gd-search-item')[1].className).toContain('active');

    fireEvent.keyDown(input, { key: 'ArrowUp' });
    expect(input).toHaveAttribute('aria-activedescendant', 'gd-search-opt-0');
  });

  it('Enter picks the active row and clears the query', () => {
    const onPick = vi.fn();
    render(<SymbolSearch packages={packages} onPick={onPick} />);
    const input = typeQuery('readfile');
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(onPick).toHaveBeenCalledWith('github.com/x/io', 'sym-ReadFile');
    expect((input as HTMLInputElement).value).toBe('');
  });

  it('Escape closes the menu, then clears the query', () => {
    render(<SymbolSearch packages={packages} onPick={() => {}} />);
    const input = typeQuery('read');
    fireEvent.keyDown(input, { key: 'Escape' });
    expect(document.querySelector('.gd-search-item')).toBeNull();
    fireEvent.keyDown(input, { key: 'Escape' });
    expect((input as HTMLInputElement).value).toBe('');
  });

  it('scopes the listbox to option rows only (kind chips live outside it)', () => {
    render(<SymbolSearch packages={packages} onPick={() => {}} />);
    typeQuery('read');
    const listbox = document.getElementById('gd-search-listbox');
    expect(listbox).toHaveAttribute('role', 'listbox');
    expect(listbox?.querySelector('.gd-search-filter')).toBeNull();
    expect(document.querySelector('.gd-search-filters')).toBeTruthy();
    // Everything the listbox owns is an option.
    for (const child of Array.from(listbox!.children)) {
      expect(['option', 'status']).toContain(child.getAttribute('role'));
    }
  });

  it('filters results by kind when a chip is toggled', () => {
    render(<SymbolSearch packages={packages} onPick={() => {}} />);
    typeQuery('lru');
    fireEvent.click(document.querySelector('.gd-search-filter[data-kind="method"]')!);
    const labels = Array.from(document.querySelectorAll('.gd-search-item-label')).map(
      (n) => n.textContent,
    );
    expect(labels).toEqual(['LRU.Get']);
  });

  it('does not crash on a package whose symbol arrays are missing', () => {
    const malformed = [
      { importPath: 'github.com/x/broken', name: 'broken', synopsis: '', doc: '' },
    ] as unknown as DocPackage[];
    expect(() => {
      render(<SymbolSearch packages={malformed} onPick={() => {}} />);
      typeQuery('broken');
    }).not.toThrow();
  });
});

describe('SymbolSearch (backend endpoint)', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ hits: [], matchedTerms: [] }),
    } as unknown as Response);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('queries the endpoint with q, first, and library params after the debounce', async () => {
    render(
      <SymbolSearch
        packages={packages}
        onPick={() => {}}
        searchEndpoint="/api/search"
        library="io-lib"
      />,
    );
    typeQuery('read');
    await act(async () => {
      await vi.advanceTimersByTimeAsync(200);
    });
    expect(global.fetch).toHaveBeenCalled();
    const url = (global.fetch as ReturnType<typeof vi.fn>).mock.calls.at(-1)?.[0] as string;
    expect(url).toContain('q=read');
    expect(url).toContain('first=12');
    expect(url).toContain('library=io-lib');
    expect(url).not.toContain('kind=');
  });

  it('appends the kind param in KIND_ORDER when kind chips are toggled', async () => {
    render(<SymbolSearch packages={packages} onPick={() => {}} searchEndpoint="/api/search" />);
    typeQuery('read');
    await act(async () => {
      await vi.advanceTimersByTimeAsync(200);
    });
    // Toggle the "type" chip first, then "func"; serialized order must follow
    // KIND_ORDER (func before type), independent of click order.
    fireEvent.click(document.querySelector('.gd-search-filter[data-kind="type"]')!);
    fireEvent.click(document.querySelector('.gd-search-filter[data-kind="func"]')!);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(200);
    });
    const url = (global.fetch as ReturnType<typeof vi.fn>).mock.calls.at(-1)?.[0] as string;
    expect(url).toContain('kind=func%2Ctype');
  });
});
