// Metadata contract for the routes: every page must carry its own canonical and
// a self-describing Open Graph / Twitter card, and the root layout must define a
// metadataBase so those relative URLs resolve absolutely.
import { describe, expect, it } from 'vitest';
import { metadata as rootMetadata } from '../../app/layout';
import { generateMetadata as libMetadata } from '../../app/lib/[id]/page';
import { metadata as parityMetadata } from '../../app/parity/page';
import { metadata as securityMetadata } from '../../app/security/page';
import { SITE_IMAGE, SITE_NAME, SITE_URL, pageMetadata } from '../../app/seo';

describe('root layout metadata', () => {
  it('declares a metadataBase so relative canonical/og URLs become absolute', () => {
    expect(rootMetadata.metadataBase?.toString()).toBe(`${SITE_URL}/`);
  });

  it('declares a site-wide Open Graph and Twitter card', () => {
    expect(rootMetadata.openGraph).toMatchObject({ type: 'website', siteName: SITE_NAME, locale: 'en_US' });
    expect(rootMetadata.twitter).toMatchObject({ card: 'summary_large_image' });
  });

  // A canonical is inherited literally by every route that does not set one, so
  // declaring one on the layout would claim each page is the home page.
  it('does not declare a canonical the child routes would inherit', () => {
    expect(rootMetadata.alternates?.canonical).toBeUndefined();
  });
});

describe('pageMetadata', () => {
  it('builds canonical, og:url and a matching card from one title/description', () => {
    const m = pageMetadata({ title: 'Title', description: 'Desc', path: '/somewhere' });
    expect(m.alternates?.canonical).toBe('/somewhere');
    expect(m.openGraph).toMatchObject({
      type: 'website',
      siteName: SITE_NAME,
      url: '/somewhere',
      title: `Title · ${SITE_NAME}`,
      description: 'Desc',
    });
    expect(m.twitter).toMatchObject({ card: 'summary_large_image', title: `Title · ${SITE_NAME}` });
  });

  // A declared openGraph replaces the root's wholesale, including the og:image
  // the layout inherited from app/opengraph-image.tsx, so it is restated.
  it('carries the site card image by default', () => {
    const m = pageMetadata({ title: 'T', description: 'D', path: '/p' });
    expect(m.openGraph).toMatchObject({ images: [{ url: SITE_IMAGE, width: 1200, height: 630 }] });
  });

  // /lib/[id] has an opengraph-image file of its own; a URL here would shadow it.
  it('omits images when the route has its own opengraph-image file', () => {
    const m = pageMetadata({ title: 'T', description: 'D', path: '/p', image: null });
    expect(m.openGraph).not.toHaveProperty('images');
    expect(m.twitter).not.toHaveProperty('images');
  });
});

describe('per-route canonicals', () => {
  it('are self-referencing on the static routes', () => {
    expect(parityMetadata.alternates?.canonical).toBe('/parity');
    expect(securityMetadata.alternates?.canonical).toBe('/security');
  });

  it('point at the library on /lib/[id]', async () => {
    const express = await libMetadata({ params: Promise.resolve({ id: 'express' }) });
    expect(express.alternates?.canonical).toBe('/lib/express');
    expect(express.openGraph).toMatchObject({ url: '/lib/express' });
    expect(express.openGraph).not.toHaveProperty('images');

    const socket = await libMetadata({ params: Promise.resolve({ id: 'socketio' }) });
    expect(socket.alternates?.canonical).toBe('/lib/socketio');
  });

  it('are omitted for an unknown library rather than pointing somewhere wrong', async () => {
    const missing = await libMetadata({ params: Promise.resolve({ id: 'nope' }) });
    expect(missing.alternates?.canonical).toBeUndefined();
    expect(missing.title).toBe('Library not found');
  });
});
