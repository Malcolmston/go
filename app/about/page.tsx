// /about — what the suite is and which libraries it covers.
import type { Metadata } from 'next';
import AboutClient from './AboutClient';
import { pageMetadata } from '../seo';

export const metadata: Metadata = pageMetadata({
  title: 'About the project',
  description:
    'What malcolmston/go is: independent Go ports of the Node.js libraries people actually reach for, one module per library, each measured against the package it replaces. Includes the full library index.',
  path: '/about',
});

export default function Page() {
  return <AboutClient />;
}
