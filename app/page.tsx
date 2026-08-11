// '/' — the landing page. A SERVER component so it can export `metadata`; the
// interactive body lives in HomeClient (see app/lib/[id]/page.tsx for the same
// split). Only the root page keeps the bare default title, since the layout's
// title.default is already written for it.
//
// It still needs its own metadata for one reason: a canonical. The layout
// deliberately declares none (inheriting one would make every route claim to be
// the home page), so without this export the home page — the highest-priority
// URL in the sitemap — is the one page with no canonical at all, leaving the
// Vercel deployment and the GitHub Pages /go export competing as duplicates.
//
// pageMetadata supplies canonical, og:url and the card; the title is then
// restated as `absolute` and the og/twitter titles as the bare site name,
// because the layout's `%s · malcolmston/go` template (and pageMetadata's own
// og:title suffix) would otherwise render "malcolmston/go · malcolmston/go".
//
// It also emits the site-level JSON-LD graph (WebSite + the collection of ports)
// built in src/components/JsonLd.tsx from LIBS, because a page file may export
// nothing but `default` and the framework's own names.
import type { Metadata } from 'next';
import HomeClient from './HomeClient';
import { JsonLd, homeGraph } from '../src/components/JsonLd';
import { SITE_DESCRIPTION, SITE_NAME, SITE_URL, pageMetadata } from './seo';

const base = pageMetadata({
  title: SITE_NAME,
  description: SITE_DESCRIPTION,
  path: '/',
});

export const metadata: Metadata = {
  ...base,
  title: { absolute: SITE_NAME },
  openGraph: { ...base.openGraph, title: SITE_NAME },
  twitter: { ...base.twitter, title: SITE_NAME },
};

export default function Page() {
  return (
    <>
      <JsonLd id="ld-home" data={homeGraph(SITE_URL, SITE_NAME, SITE_DESCRIPTION)} />
      <HomeClient />
    </>
  );
}
