// /faq — the recurring questions about the ports.
//
// Besides the metadata, this route publishes the FAQPage structured data: the
// graph is generated in src/components/JsonLd.tsx from the same FAQS array the
// accordion renders (a page file may export nothing but `default` and the
// framework's own names, so the builder cannot live here).
import type { Metadata } from 'next';
import FaqClient from './FaqClient';
import { JsonLd, faqGraph } from '../../src/components/JsonLd';
import { SITE_NAME, SITE_URL, pageMetadata } from '../seo';

const TITLE = 'Frequently asked questions';

export const metadata: Metadata = pageMetadata({
  title: TITLE,
  description:
    'Answers to the questions the Go ports raise most often: how close parity really is, how the numbers are measured, versioning and stability, and when you should not use a port.',
  path: '/faq',
});

export default function Page() {
  return (
    <>
      <JsonLd id="ld-faq" data={faqGraph(SITE_URL, `${TITLE} · ${SITE_NAME}`)} />
      <FaqClient />
    </>
  );
}
