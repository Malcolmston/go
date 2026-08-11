// /releases — the published version of every module.
import type { Metadata } from 'next';
import ReleasesClient from './ReleasesClient';
import { pageMetadata } from '../seo';

export const metadata: Metadata = pageMetadata({
  title: 'Releases',
  description:
    'The latest published tag for every module in the suite, with the release notes for each, so you can see what changed in a port before you upgrade.',
  path: '/releases',
});

export default function Page() {
  return <ReleasesClient />;
}
