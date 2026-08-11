// Metadata for the pages that report "this thing does not exist": the app-wide
// 404 (app/not-found.tsx) and the unresolved-id branches of /lib/[id] and
// /parity/[lib].
//
// A bare `return { title: 'Library not found' }` is not enough. Next merges
// metadata shallowly per top-level key, so a route that declares only `title`
// keeps the ROOT layout's `openGraph` — whose url is '/' and whose title is the
// site name. A dead end then previews on Slack or Twitter as the home page, and
// Google, seeing a 200-or-404 body with the home page's og:url and no robots
// directive, treats it as a thin indexable duplicate.
//
// So this helper restates openGraph/twitter with no `url` at all (nothing to be
// canonical about) and marks the page noindex. `follow` stays true: the recovery
// links on those pages are real, and a crawler is welcome to walk them back into
// the site.
import type { Metadata } from 'next';
import { SITE_NAME } from './seo';

export function notFoundMetadata({
  title,
  description,
}: {
  title: string;
  description: string;
}): Metadata {
  const ogTitle = `${title} · ${SITE_NAME}`;
  return {
    title,
    description,
    // No `alternates.canonical`: a page that does not exist has no canonical
    // URL, and pointing one at itself would invite indexing.
    robots: { index: false, follow: true },
    openGraph: {
      type: 'website',
      siteName: SITE_NAME,
      locale: 'en_US',
      title: ogTitle,
      description,
    },
    twitter: {
      card: 'summary',
      title: ogTitle,
      description,
    },
  };
}
