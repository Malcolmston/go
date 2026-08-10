package com.malcolmston.parity;

import com.fasterxml.jackson.databind.JsonNode;
import org.apache.lucene.analysis.Analyzer;
import org.apache.lucene.document.Document;
import org.apache.lucene.document.Field;
import org.apache.lucene.document.StoredField;
import org.apache.lucene.document.TextField;
import org.apache.lucene.index.DirectoryReader;
import org.apache.lucene.index.IndexWriter;
import org.apache.lucene.index.IndexWriterConfig;
import org.apache.lucene.search.IndexSearcher;
import org.apache.lucene.search.similarities.BM25Similarity;
import org.apache.lucene.store.ByteBuffersDirectory;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * Holds the two RAM indexes built from the shared fixture corpus: "stop" (no
 * stemming) and "standard" (Porter stemming). Documents are added in fixture
 * order so that internal doc ids, and therefore tie-breaks, are reproducible.
 */
final class Indexes {
  final DirectoryReader reader;
  final IndexSearcher searcher;
  final Analyzer analyzer;

  private static final Map<String, Indexes> BY_NAME = new HashMap<>();
  private static List<String> fieldNames;

  private Indexes(DirectoryReader r, Analyzer a) {
    this.reader = r;
    this.analyzer = a;
    this.searcher = new IndexSearcher(r);
    this.searcher.setSimilarity(new BM25Similarity());
  }

  static Indexes get(String name) {
    Indexes ix = BY_NAME.get(name);
    if (ix == null) {
      throw new IllegalArgumentException("unknown index: " + name);
    }
    return ix;
  }

  static void build(JsonNode fixture) throws Exception {
    List<String> stops = new ArrayList<>();
    for (JsonNode w : fixture.get("stopWords")) {
      stops.add(w.asText());
    }
    Analyzers.setStopWords(stops);

    fieldNames = new ArrayList<>();
    for (JsonNode f : fixture.get("fields")) {
      fieldNames.add(f.asText());
    }

    BY_NAME.put("stop", make(Analyzers.of("stop"), fixture));
    BY_NAME.put("standard", make(Analyzers.of("standard"), fixture));
  }

  /** Sorted, matching the port's Index.Fields contract. */
  static List<String> fields() {
    List<String> f = new ArrayList<>(fieldNames);
    java.util.Collections.sort(f);
    return f;
  }

  private static Indexes make(Analyzer a, JsonNode fixture) throws Exception {
    ByteBuffersDirectory dir = new ByteBuffersDirectory();
    IndexWriterConfig cfg = new IndexWriterConfig(a);
    cfg.setOpenMode(IndexWriterConfig.OpenMode.CREATE);
    try (IndexWriter w = new IndexWriter(dir, cfg)) {
      for (JsonNode d : fixture.get("docs")) {
        Document doc = new Document();
        doc.add(new StoredField("id", d.get("id").asText()));
        for (String f : fieldNames) {
          doc.add(new Field(f, d.get(f).asText(), TextField.TYPE_NOT_STORED));
        }
        w.addDocument(doc);
      }
      w.forceMerge(1);
    }
    return new Indexes(DirectoryReader.open(dir), a);
  }

  String docId(int docNum) throws Exception {
    return searcher.storedFields().document(docNum).get("id");
  }
}
