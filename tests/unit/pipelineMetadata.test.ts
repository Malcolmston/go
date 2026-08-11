import { describe, it, expect } from 'vitest';
import { metadata as pipelineMetadata } from '../../app/pipeline/page';
import { SITE_NAME } from '../../app/seo';

// /pipeline is a redirect stub kept alive for old inbound links. It is a server
// component only so that it can declare the metadata below: without it the route
// inherited the layout defaults, which meant the home page's <title>, no
// canonical, and og:url at the site root. These assertions are the guard that a
// future edit does not turn it back into a bare 'use client' page.
describe('/pipeline metadata', () => {
  it('does not reuse the site-wide title', () => {
    expect(pipelineMetadata.title).toBe('The pipeline moved');
    expect(pipelineMetadata.title).not.toBe(SITE_NAME);
  });

  it('canonicalises at /parity, the destination of the redirect', () => {
    expect(pipelineMetadata.alternates?.canonical).toBe('/parity');
    expect(pipelineMetadata.openGraph).toMatchObject({ url: '/parity' });
  });

  it('asks crawlers not to index the stub but to follow it through', () => {
    expect(pipelineMetadata.robots).toEqual({ index: false, follow: true });
  });
});
