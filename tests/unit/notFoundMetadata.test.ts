import { describe, it, expect } from 'vitest';
import type { Metadata } from 'next';
import { notFoundMetadata } from '../../app/notFoundMetadata';
import { metadata as notFoundPageMetadata } from '../../app/not-found';
import { generateMetadata as libMetadata } from '../../app/lib/[id]/page';
import { aliasFor } from '../../app/lib/[id]/alias';
import { generateMetadata as parityMetadata } from '../../app/parity/[lib]/page';

// Regression: the "not found" branches used to return a bare { title }. Next
// merges metadata shallowly per top-level key, so those pages kept the ROOT
// layout's openGraph — url '/' and the site title — and had no robots
// directive. Every typo'd /lib/<id> link then previewed as the home page and was
// indexable as a thin duplicate.
function og(meta: Metadata) {
  return (meta.openGraph ?? {}) as Record<string, unknown>;
}

describe('notFoundMetadata', () => {
  it('marks the page noindex but still followable', () => {
    const meta = notFoundMetadata({ title: 'Nope', description: 'Gone.' });
    expect(meta.robots).toEqual({ index: false, follow: true });
  });

  it('declares its own openGraph with no url, so the home og:url is not inherited', () => {
    const meta = notFoundMetadata({ title: 'Nope', description: 'Gone.' });
    expect(og(meta).url).toBeUndefined();
    expect(og(meta).title).toBe('Nope · malcolmston/go');
    expect(og(meta).siteName).toBe('malcolmston/go');
  });

  it('claims no canonical for a page that does not exist', () => {
    const meta = notFoundMetadata({ title: 'Nope', description: 'Gone.' });
    expect(meta.alternates?.canonical).toBeUndefined();
  });
});

describe('the routes that can miss', () => {
  it('app/not-found is noindex', () => {
    expect(notFoundPageMetadata.robots).toEqual({ index: false, follow: true });
    expect(og(notFoundPageMetadata).url).toBeUndefined();
  });

  it('an unknown /lib/<id> is noindex with no inherited og:url', async () => {
    const meta = await libMetadata({ params: Promise.resolve({ id: 'not-a-library' }) });
    expect(meta.title).toBe('Library not found');
    expect(meta.robots).toEqual({ index: false, follow: true });
    expect(og(meta).url).toBeUndefined();
    expect(meta.alternates?.canonical).toBeUndefined();
  });

  it('an unknown /parity/<slug> is noindex with no inherited og:url', async () => {
    const meta = await parityMetadata({ params: Promise.resolve({ lib: 'not-a-harness' }) });
    expect(meta.title).toBe('Parity record not found');
    expect(meta.robots).toEqual({ index: false, follow: true });
    expect(og(meta).url).toBeUndefined();
    expect(meta.alternates?.canonical).toBeUndefined();
  });

  it('a real library keeps its indexable canonical metadata', async () => {
    const meta = await libMetadata({ params: Promise.resolve({ id: 'express' }) });
    expect(meta.robots).toBeUndefined();
    expect(meta.alternates?.canonical).toBe('/lib/express');
  });
});

describe('aliasFor', () => {
  it('maps the dotted repo spelling to the canonical library id', () => {
    expect(aliasFor('socket.io')).toBe('socketio');
  });

  it('leaves real ids alone and refuses to invent one', () => {
    expect(aliasFor('socketio')).toBeNull();
    expect(aliasFor('express')).toBeNull();
    expect(aliasFor('not-a-library')).toBeNull();
  });
});
