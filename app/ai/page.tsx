// /ai — how the ports were written with AI in the loop.
import type { Metadata } from 'next';
import AiClient from './AiClient';
import { pageMetadata } from '../seo';

export const metadata: Metadata = pageMetadata({
  title: 'How the ports were built with AI',
  description:
    'The working method behind the Go ports: what the model wrote, what the parity harness had to prove before anything shipped, and where human review stayed in the loop.',
  path: '/ai',
});

export default function Page() {
  return <AiClient />;
}
