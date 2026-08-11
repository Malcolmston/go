// Structured-data contract: the pages publish JSON-LD that parses, that names
// the canonical origin, and that is DERIVED from the same data the pages render
// (LIBS and FAQS) rather than hand-copied — the drift this guards against is a
// crawler being told about libraries or answers the site no longer shows.
import { describe, expect, it } from 'vitest';
import { faqGraph, homeGraph, plainText, serializeJsonLd } from '../../src/components/JsonLd';
import { FAQS, LIBS } from '../../src/data';
import { SITE_DESCRIPTION, SITE_NAME, SITE_URL } from '../../app/seo';

// The builders take the origin as an argument (nothing under src/ imports app/),
// and the routes pass SITE_URL — the constant metadataBase already resolves every
// canonical against, so the graph and the canonical always name the same host.
const ORIGIN = SITE_URL.replace(/\/+$/, '');
const FAQ_NAME = `Frequently asked questions · ${SITE_NAME}`;
const home = () => homeGraph(SITE_URL, SITE_NAME, SITE_DESCRIPTION);
const faq = () => faqGraph(SITE_URL, FAQ_NAME);

describe('serializeJsonLd', () => {
  it('round-trips through JSON.parse unchanged', () => {
    const data = { a: 1, b: 'plain', c: [true, null] };
    expect(JSON.parse(serializeJsonLd(data))).toEqual(data);
  });

  // A raw </script> in any string value would close the element early and spill
  // the remaining JSON into the document as markup.
  it('escapes the characters that could break out of a <script> element', () => {
    const out = serializeJsonLd({ text: '</script><img src=x> & more' });
    expect(out).not.toMatch(/[<>&]/);
    expect(out).toContain('\\u003c');
    expect(JSON.parse(out).text).toBe('</script><img src=x> & more');
  });

  it('escapes the JS line terminators that are legal inside JSON', () => {
    const out = serializeJsonLd({ text: `a\u2028b\u2029c` });
    expect(out).not.toMatch(/[\u2028\u2029]/);
    expect(JSON.parse(out).text).toBe('a\u2028b\u2029c');
  });
});

describe('plainText', () => {
  it('reduces authored markup to the text a reader sees', () => {
    expect(plainText('Handlers with the <code>(req, res, next)</code> shape')).toBe(
      'Handlers with the (req, res, next) shape',
    );
    expect(plainText('SSE &amp; chunked streaming')).toBe('SSE & chunked streaming');
    expect(plainText('a &lt;b&gt;   c')).toBe('a <b> c');
  });
});

describe('home graph', () => {
  const graph = home() as { '@graph': Record<string, unknown>[] };
  const nodes = graph['@graph'];
  const site = nodes.find((n) => n['@type'] === 'WebSite')!;
  const list = nodes.find((n) => n['@type'] === 'ItemList')!;

  it('serialises to parseable JSON-LD with a schema.org context', () => {
    const parsed = JSON.parse(serializeJsonLd(home()));
    expect(parsed['@context']).toBe('https://schema.org');
    expect(Array.isArray(parsed['@graph'])).toBe(true);
  });

  it('describes the site from the shared SEO constants', () => {
    expect(site.name).toBe(SITE_NAME);
    expect(site.description).toBe(SITE_DESCRIPTION);
    expect(site['@id']).toBe(`${ORIGIN}/#website`);
    expect(site.url).toBe(`${ORIGIN}/`);
  });

  it('lists every library in LIBS, in order, with no hand-written count', () => {
    const items = list.itemListElement as { position: number; item: Record<string, string> }[];
    expect(list.numberOfItems).toBe(LIBS.length);
    expect(items).toHaveLength(LIBS.length);
    expect(items.map((e) => e.position)).toEqual(LIBS.map((_, i) => i + 1));
    expect(items.map((e) => e.item.name)).toEqual(LIBS.map((l) => l.name));
  });

  it('gives each port a SoftwareSourceCode node pointing at its own page and repo', () => {
    const items = list.itemListElement as { item: Record<string, string> }[];
    for (const [i, { item }] of items.entries()) {
      const lib = LIBS[i];
      expect(item['@type']).toBe('SoftwareSourceCode');
      expect(item.url).toBe(`${ORIGIN}/lib/${lib.id}`);
      expect(item.codeRepository).toBe(lib.repo);
      expect(item.identifier).toBe(lib.pkg);
      expect(item.programmingLanguage).toBe('Go');
      expect(item.description).toBe(plainText(lib.tagline));
    }
  });

  // A trailing slash on NEXT_PUBLIC_SITE_URL must not produce '//#website'.
  it('normalises a trailing slash on the origin it is given', () => {
    const g = homeGraph(`${ORIGIN}/`, SITE_NAME, SITE_DESCRIPTION) as { '@graph': Record<string, string>[] };
    expect(g['@graph'][0]['@id']).toBe(`${ORIGIN}/#website`);
  });

  // /explore has no ?q handling, so a SearchAction here would advertise a URL
  // template that does not resolve.
  it('advertises no search action', () => {
    expect(JSON.stringify(home())).not.toContain('SearchAction');
  });
});

describe('faq graph', () => {
  const graph = faq() as Record<string, unknown>;

  it('is an FAQPage at the canonical /faq URL', () => {
    expect(graph['@type']).toBe('FAQPage');
    expect(graph.url).toBe(`${ORIGIN}/faq`);
    expect(graph.name).toBe(FAQ_NAME);
    expect(graph.isPartOf).toEqual({ '@id': `${ORIGIN}/#website` });
  });

  it('mirrors FAQS exactly, questions and answers, as text', () => {
    const questions = graph.mainEntity as { '@type': string; name: string; acceptedAnswer: { '@type': string; text: string } }[];
    expect(questions).toHaveLength(FAQS.length);
    expect(questions.map((q) => q.name)).toEqual(FAQS.map(([q]) => plainText(q)));
    for (const [i, q] of questions.entries()) {
      expect(q['@type']).toBe('Question');
      expect(q.acceptedAnswer['@type']).toBe('Answer');
      expect(q.acceptedAnswer.text).toBe(plainText(FAQS[i][1]));
      expect(q.acceptedAnswer.text).not.toMatch(/[<>]/);
    }
  });

  it('serialises to parseable JSON-LD', () => {
    expect(JSON.parse(serializeJsonLd(faq()))['@type']).toBe('FAQPage');
  });
});
