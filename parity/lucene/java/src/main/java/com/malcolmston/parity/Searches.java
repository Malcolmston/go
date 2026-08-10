package com.malcolmston.parity;

import org.apache.lucene.index.MultiTerms;
import org.apache.lucene.index.Term;
import org.apache.lucene.index.Terms;
import org.apache.lucene.index.TermsEnum;
import org.apache.lucene.search.Query;
import org.apache.lucene.search.ScoreDoc;
import org.apache.lucene.search.TopDocs;
import org.apache.lucene.util.BytesRef;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Map;

/**
 * Runs a query and reduces the answer to what is comparable across independent
 * implementations: the number of matches and the doc ids. Absolute BM25 scores
 * are never emitted; only the ranked ORDER is, with equal-scoring ids sorted so
 * that an arbitrary intra-tie ordering cannot masquerade as a divergence.
 */
final class Searches {
  private Searches() {}

  static Map<String, Object> ranked(Indexes ix, Query q, int topN) throws Exception {
    int total = ix.searcher.count(q);
    TopDocs td = ix.searcher.search(q, Math.max(topN, 1));
    List<String> ids = new ArrayList<>();
    List<String> tie = new ArrayList<>();
    float last = Float.NaN;
    for (ScoreDoc sd : td.scoreDocs) {
      float s = round(sd.score);
      if (!tie.isEmpty() && s != last) {
        Collections.sort(tie);
        ids.addAll(tie);
        tie.clear();
      }
      last = s;
      tie.add(ix.docId(sd.doc));
    }
    Collections.sort(tie);
    ids.addAll(tie);
    if (topN > 0 && ids.size() > topN) {
      ids = ids.subList(0, topN);
    }
    return Json.hits(total, ids);
  }

  static Map<String, Object> set(Indexes ix, Query q, int topN) throws Exception {
    int total = ix.searcher.count(q);
    TopDocs td = ix.searcher.search(q, Math.max(topN, 1));
    List<String> ids = new ArrayList<>();
    for (ScoreDoc sd : td.scoreDocs) {
      ids.add(ix.docId(sd.doc));
    }
    Collections.sort(ids);
    return Json.hits(total, ids);
  }

  /** Six decimal places: enough to separate real score differences, blind to float noise. */
  private static float round(float f) {
    return Math.round(f * 1e6f) / 1e6f;
  }

  static List<String> terms(Indexes ix, String field) throws Exception {
    List<String> out = new ArrayList<>();
    Terms t = MultiTerms.getTerms(ix.reader, field);
    if (t == null) {
      return out;
    }
    TermsEnum e = t.iterator();
    BytesRef b;
    while ((b = e.next()) != null) {
      out.add(b.utf8ToString());
    }
    Collections.sort(out);
    return out;
  }

  static int docFreq(Indexes ix, String field, String term) throws Exception {
    String a = Analyzers.one(ix.analyzer, term);
    if (a.isEmpty()) {
      return 0;
    }
    return ix.reader.docFreq(new Term(field, a));
  }

  static long totalTermFreq(Indexes ix, String field, String term) throws Exception {
    String a = Analyzers.one(ix.analyzer, term);
    if (a.isEmpty()) {
      return 0;
    }
    return ix.reader.totalTermFreq(new Term(field, a));
  }
}
