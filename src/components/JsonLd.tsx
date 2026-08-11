// JsonLd emits a <script type="application/ld+json"> block from a plain object.
//
// Structured data is the one part of the page a crawler reads instead of the
// markup, and the site had none: the FAQ was a genuine question-and-answer page
// and every /lib page a described software package, but both were invisible to
// rich results. This component is the single place that turns a JS object into
// the script tag, so no route has to hand-write JSON in a template string.
//
// Zero dependency on purpose (see the repo's no-new-deps rule): the payload is
// JSON.stringify plus the four-character escape that makes the result safe
// inside an HTML <script> element.
import type { ReactElement } from 'react';
import { FAQS, LIBS } from '../data';

/** Anything JSON.stringify can represent. `undefined` members are dropped. */
export type JsonLdData = Record<string, unknown>;

/**
 * Serialise a JSON-LD payload for embedding in <script type="application/ld+json">.
 *
 * The escaping is not cosmetic. A raw `</script>` (or a lone `<`/`>`) inside a
 * string value would terminate the element early and spill the rest of the JSON
 * into the document as markup, and `&` can begin an entity a parser rewrites, so
 * all three are emitted as \uXXXX — which JSON readers unescape back to the same
 * characters, leaving the parsed object identical. U+2028/U+2029 are legal in
 * JSON but are line terminators to a JS parser, so they go too.
 *
 * Exported separately from the component so a unit test (and any future
 * non-React caller) can assert the exact bytes.
 */
export function serializeJsonLd(data: JsonLdData): string {
  return JSON.stringify(data)
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/&/g, '\\u0026')
    .replace(/\u2028/g, '\\u2028')
    .replace(/\u2029/g, '\\u2029');
}

/**
 * Strip authored markup out of a content string so it can be used as a JSON-LD
 * text value.
 *
 * The prose in src/data.ts is written for dangerouslySetInnerHTML (the Html
 * component), so an answer may legitimately contain <code> tags and entities.
 * Structured data wants the text a reader would see, and deriving it here is
 * what keeps the markup and the JSON-LD from drifting apart: the routes pass the
 * SAME strings the page renders.
 */
export function plainText(html: string): string {
  return html
    .replace(/<[^>]*>/g, '')
    .replace(/&nbsp;/g, ' ')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&#0*39;/g, "'")
    .replace(/&apos;/g, "'")
    // Last, so an escaped entity like &amp;lt; does not become a tag delimiter.
    .replace(/&amp;/g, '&')
    .replace(/\s+/g, ' ')
    .trim();
}

/**
 * The canonical origin, without a trailing slash. The routes pass SITE_URL from
 * app/seo.ts — the same constant metadataBase resolves every canonical against —
 * so a JSON-LD @id can never name a different host than the page's canonical.
 * It is a parameter rather than an import because nothing under src/ depends on
 * app/, and this file is imported by the client-rendered views' neighbours.
 */
function origin(siteUrl: string): string {
  return siteUrl.replace(/\/+$/, '');
}

/**
 * The home page's graph: the site entity plus the collection of ports, so a
 * crawler learns what this site is and that it publishes N named software
 * packages rather than N unrelated URLs.
 *
 * Derived from LIBS — the same array the cards on the page render — so adding a
 * library updates the graph with no edit here. There is deliberately no
 * SearchAction: /explore accepts no ?q parameter, and advertising a query
 * template that does not resolve is worse than advertising none.
 *
 * The builders live here, beside the serialiser, because a Next page file may
 * only export `default` and the framework's own names — an extra export from
 * app/page.tsx is a type error in .next/types.
 */
export function homeGraph(siteUrl: string, name: string, description: string): JsonLdData {
  const base = origin(siteUrl);
  const website = `${base}/#website`;
  return {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'WebSite',
        '@id': website,
        url: `${base}/`,
        name,
        description,
        inLanguage: 'en',
      },
      {
        '@type': 'ItemList',
        '@id': `${base}/#libraries`,
        name: 'Go ports of the Node.js and Python libraries',
        isPartOf: { '@id': website },
        numberOfItems: LIBS.length,
        itemListElement: LIBS.map((lib, i) => ({
          '@type': 'ListItem',
          position: i + 1,
          item: {
            '@type': 'SoftwareSourceCode',
            '@id': `${base}/lib/${lib.id}#software`,
            name: lib.name,
            url: `${base}/lib/${lib.id}`,
            description: plainText(lib.tagline),
            programmingLanguage: 'Go',
            codeRepository: lib.repo,
            // The Go module path is how the package is actually installed, and
            // the generated API reference is the same entity on another host.
            identifier: lib.pkg,
            sameAs: lib.docs,
            license: 'https://opensource.org/licenses/MIT',
            isPartOf: { '@id': website },
          },
        })),
      },
    ],
  };
}

/**
 * The FAQPage graph for /faq.
 *
 * Generated from FAQS — the exact array src/components/Faq.tsx renders — rather
 * than hand-copied, so the accordion a reader sees and the questions a crawler
 * reads cannot drift apart.
 */
export function faqGraph(siteUrl: string, pageName: string): JsonLdData {
  const base = origin(siteUrl);
  return {
    '@context': 'https://schema.org',
    '@type': 'FAQPage',
    '@id': `${base}/faq#faqpage`,
    url: `${base}/faq`,
    name: pageName,
    inLanguage: 'en',
    isPartOf: { '@id': `${base}/#website` },
    mainEntity: FAQS.map(([question, answer]) => ({
      '@type': 'Question',
      name: plainText(question),
      acceptedAnswer: { '@type': 'Answer', text: plainText(answer) },
    })),
  };
}

export interface JsonLdProps {
  /** The graph or single node to publish. */
  data: JsonLdData;
  /**
   * Optional element id, useful when a page emits more than one block and a
   * test (or a person reading view-source) needs to tell them apart.
   */
  id?: string;
}

export function JsonLd({ data, id }: JsonLdProps): ReactElement {
  return (
    <script
      type="application/ld+json"
      {...(id ? { id } : {})}
      dangerouslySetInnerHTML={{ __html: serializeJsonLd(data) }}
    />
  );
}
