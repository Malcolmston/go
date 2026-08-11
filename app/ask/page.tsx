// /ask — the chat interface over the suite's own documentation.
import type { Metadata } from 'next';
import AskClient from './AskClient';
import { pageMetadata } from '../seo';

export const metadata: Metadata = pageMetadata({
  title: 'Ask about the libraries',
  description:
    'Ask a question about any of the Go ports and get an answer grounded in the indexed documentation and exported symbols of this suite, with links to the pages it drew on.',
  path: '/ask',
});

export default function Page() {
  return <AskClient />;
}
