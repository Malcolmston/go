package com.malcolmston.parity;

import org.apache.lucene.analysis.TokenStream;
import org.apache.lucene.analysis.core.WhitespaceTokenizer;
import org.apache.lucene.analysis.ngram.EdgeNGramTokenizer;
import org.apache.lucene.analysis.ngram.NGramTokenizer;
import org.apache.lucene.analysis.shingle.ShingleFilter;
import org.apache.lucene.analysis.tokenattributes.CharTermAttribute;
import org.apache.lucene.index.Term;
import org.apache.lucene.search.WildcardQuery;
import org.apache.lucene.util.automaton.Automaton;
import org.apache.lucene.util.automaton.CharacterRunAutomaton;
import org.apache.lucene.util.automaton.LevenshteinAutomata;

import java.io.StringReader;
import java.util.ArrayList;
import java.util.List;

/** Upstream counterparts of the port's standalone helper functions. */
final class Utils {
  private Utils() {}

  private static List<String> drain(TokenStream ts) throws Exception {
    List<String> out = new ArrayList<>();
    CharTermAttribute term = ts.addAttribute(CharTermAttribute.class);
    ts.reset();
    while (ts.incrementToken()) {
      out.add(term.toString());
    }
    ts.end();
    ts.close();
    return out;
  }

  static List<String> ngrams(String s, int n) throws Exception {
    NGramTokenizer t = new NGramTokenizer(n, n);
    t.setReader(new StringReader(s));
    return drain(t);
  }

  static List<String> edgeNgrams(String s, int min, int max) throws Exception {
    EdgeNGramTokenizer t = new EdgeNGramTokenizer(min, max);
    t.setReader(new StringReader(s));
    return drain(t);
  }

  static List<String> shingles(List<String> tokens, int n) throws Exception {
    WhitespaceTokenizer src = new WhitespaceTokenizer();
    src.setReader(new StringReader(String.join(" ", tokens)));
    ShingleFilter f = new ShingleFilter(src, n, n);
    f.setOutputUnigrams(false);
    return drain(f);
  }

  static boolean wildcardMatch(String pattern, String s) {
    WildcardQuery q = new WildcardQuery(new Term("text", pattern));
    return new CharacterRunAutomaton(q.getAutomaton()).run(s);
  }

  /** True when b is within n edits of a, as FuzzyQuery decides it (no transpositions). */
  static boolean withinEdits(String a, String b, int n) {
    if (n > LevenshteinAutomata.MAXIMUM_SUPPORTED_DISTANCE) {
      throw new IllegalArgumentException(
          "max edits must be <= " + LevenshteinAutomata.MAXIMUM_SUPPORTED_DISTANCE);
    }
    Automaton aut = new LevenshteinAutomata(a, false).toAutomaton(n);
    if (aut == null) {
      throw new IllegalArgumentException("no automaton for n=" + n);
    }
    return new CharacterRunAutomaton(aut).run(b);
  }
}
