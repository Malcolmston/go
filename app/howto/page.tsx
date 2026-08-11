// /howto — getting started with any of the modules.
import type { Metadata } from 'next';
import HowtoClient from './HowtoClient';
import { pageMetadata } from '../seo';

export const metadata: Metadata = pageMetadata({
  title: 'Getting started',
  description:
    'Install and use any library in the suite: the go get line for each module, a first working example, and the conventions the ports share when translating a Node.js API to Go.',
  path: '/howto',
});

export default function Page() {
  return <HowtoClient />;
}
