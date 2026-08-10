package com.malcolmston.parity;

import org.apache.lucene.analysis.Analyzer;
import org.apache.lucene.analysis.LowerCaseFilter;
import org.apache.lucene.analysis.StopFilter;
import org.apache.lucene.analysis.TokenStream;
import org.apache.lucene.analysis.CharArraySet;
import org.apache.lucene.analysis.en.EnglishAnalyzer;
import org.apache.lucene.analysis.en.PorterStemFilter;
import org.apache.lucene.analysis.standard.StandardTokenizer;
import org.apache.lucene.analysis.tokenattributes.CharTermAttribute;
import org.apache.lucene.analysis.tokenattributes.PositionIncrementAttribute;

import java.io.IOException;
import java.util.ArrayList;
import java.util.List;

/**
 * Builds the four analyzer pipelines the case files name, plus the token-stream
 * helpers. The pipelines are chosen to be the closest upstream equivalent of the
 * port's configurable Analyzer, so any token difference is a real difference.
 */
final class Analyzers {
  private static CharArraySet stopSet;

  private Analyzers() {}

  static void setStopWords(List<String> words) {
    stopSet = new CharArraySet(words, true);
  }

  static Analyzer of(String name) {
    switch (name) {
      case "lower":
        return chain(false, false);
      case "stop":
        return chain(true, false);
      case "porter":
        return chain(false, true);
      case "standard":
        return chain(true, true);
      case "english":
        return new EnglishAnalyzer();
      default:
        throw new IllegalArgumentException("unknown analyzer: " + name);
    }
  }

  private static Analyzer chain(boolean stop, boolean stem) {
    return new Analyzer() {
      @Override
      protected TokenStreamComponents createComponents(String field) {
        StandardTokenizer src = new StandardTokenizer();
        TokenStream ts = new LowerCaseFilter(src);
        if (stop) {
          ts = new StopFilter(ts, stopSet);
        }
        if (stem) {
          ts = new PorterStemFilter(ts);
        }
        return new TokenStreamComponents(src, ts);
      }
    };
  }

  static List<String> tokens(Analyzer a, String text) throws IOException {
    List<String> out = new ArrayList<>();
    try (TokenStream ts = a.tokenStream("text", text)) {
      CharTermAttribute term = ts.addAttribute(CharTermAttribute.class);
      ts.reset();
      while (ts.incrementToken()) {
        out.add(term.toString());
      }
      ts.end();
    }
    return out;
  }

  /** "position:token" strings, position being the cumulative posIncr minus one. */
  static List<String> positions(Analyzer a, String text) throws IOException {
    List<String> out = new ArrayList<>();
    try (TokenStream ts = a.tokenStream("text", text)) {
      CharTermAttribute term = ts.addAttribute(CharTermAttribute.class);
      PositionIncrementAttribute inc = ts.addAttribute(PositionIncrementAttribute.class);
      ts.reset();
      int pos = -1;
      while (ts.incrementToken()) {
        pos += inc.getPositionIncrement();
        out.add(pos + ":" + term.toString());
      }
      ts.end();
    }
    return out;
  }

  /** Single-token normalisation of a query term, or the empty string. */
  static String one(Analyzer a, String text) throws IOException {
    List<String> t = tokens(a, text);
    return t.isEmpty() ? "" : t.get(0);
  }
}
