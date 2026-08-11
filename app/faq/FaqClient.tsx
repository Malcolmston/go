'use client';
import { Faq } from '../../src/components/Faq';

// Client half of /faq. The route is a server component (so it can export
// `metadata`); this file carries the 'use client' boundary that the view needs.
export default function FaqClient() {
  return <Faq />;
}
