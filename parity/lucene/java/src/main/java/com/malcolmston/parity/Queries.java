package com.malcolmston.parity;

import com.fasterxml.jackson.databind.JsonNode;
import org.apache.lucene.index.Term;
import org.apache.lucene.search.BooleanClause;
import org.apache.lucene.search.BooleanQuery;
import org.apache.lucene.search.BoostQuery;
import org.apache.lucene.search.ConstantScoreQuery;
import org.apache.lucene.search.DisjunctionMaxQuery;
import org.apache.lucene.search.FuzzyQuery;
import org.apache.lucene.search.MatchAllDocsQuery;
import org.apache.lucene.search.PhraseQuery;
import org.apache.lucene.search.PrefixQuery;
import org.apache.lucene.search.Query;
import org.apache.lucene.search.RegexpQuery;
import org.apache.lucene.search.TermInSetQuery;
import org.apache.lucene.search.TermQuery;
import org.apache.lucene.search.TermRangeQuery;
import org.apache.lucene.util.BytesRef;

import java.util.ArrayList;
import java.util.List;

/** Translates the JSON query descriptions in cases/query.json into real Lucene queries. */
final class Queries {
  private Queries() {}

  static Query build(JsonNode q, Indexes ix) throws Exception {
    String type = Json.str(q, "type", "");
    String field = Json.str(q, "field", "text");
    switch (type) {
      case "matchall":
        return new MatchAllDocsQuery();
      case "term":
        return new TermQuery(new Term(field, analyzed(q, "term", ix)));
      case "terms":
        return termsQuery(q, field, ix);
      case "phrase":
        return phrase(q, field, ix);
      case "prefix":
        return new PrefixQuery(new Term(field, Json.str(q, "prefix", "")));
      case "wildcard":
        return new org.apache.lucene.search.WildcardQuery(
            new Term(field, Json.str(q, "pattern", "")));
      case "regexp":
        return new RegexpQuery(new Term(field, Json.str(q, "pattern", "")));
      case "fuzzy":
        return new FuzzyQuery(
            new Term(field, Json.str(q, "term", "")), Json.num(q, "maxEdits", 2));
      case "range":
        return TermRangeQuery.newStringRange(
            field,
            Json.orNull(Json.str(q, "lower", "")),
            Json.orNull(Json.str(q, "upper", "")),
            Json.bool(q, "includeLower", true),
            Json.bool(q, "includeUpper", true));
      case "boost":
        return new BoostQuery(build(q.get("query"), ix), (float) Json.dbl(q, "boost", 1));
      case "constant":
        return constant(q, ix);
      case "dismax":
        return dismax(q, ix);
      case "bool":
        return bool(q, ix);
      default:
        throw new IllegalArgumentException("unknown query type: " + type);
    }
  }

  private static String lower(String s) {
    return s.toLowerCase(java.util.Locale.ROOT);
  }

  /** A query term goes through the index analyzer, exactly as the port does. */
  private static String analyzed(JsonNode q, String key, Indexes ix) throws Exception {
    String raw = Json.str(q, key, "");
    String t = Analyzers.one(ix.analyzer, raw);
    return t.isEmpty() ? lower(raw) : t;
  }

  private static Query termsQuery(JsonNode q, String field, Indexes ix) throws Exception {
    List<BytesRef> refs = new ArrayList<>();
    for (JsonNode t : q.get("terms")) {
      String a = Analyzers.one(ix.analyzer, t.asText());
      refs.add(new BytesRef(a.isEmpty() ? lower(t.asText()) : a));
    }
    return new TermInSetQuery(field, refs);
  }

  private static Query phrase(JsonNode q, String field, Indexes ix) throws Exception {
    PhraseQuery.Builder b = new PhraseQuery.Builder();
    int pos = 0;
    for (JsonNode t : q.get("terms")) {
      String a = Analyzers.one(ix.analyzer, t.asText());
      b.add(new Term(field, a.isEmpty() ? lower(t.asText()) : a), pos);
      pos++;
    }
    return b.build();
  }

  private static Query constant(JsonNode q, Indexes ix) throws Exception {
    ConstantScoreQuery c = new ConstantScoreQuery(build(q.get("query"), ix));
    double s = Json.dbl(q, "score", 1);
    return s == 1.0 ? c : new BoostQuery(c, (float) s);
  }

  private static Query dismax(JsonNode q, Indexes ix) throws Exception {
    List<Query> subs = new ArrayList<>();
    for (JsonNode d : q.get("disjuncts")) {
      subs.add(build(d, ix));
    }
    return new DisjunctionMaxQuery(subs, (float) Json.dbl(q, "tieBreaker", 0));
  }

  private static Query bool(JsonNode q, Indexes ix) throws Exception {
    BooleanQuery.Builder b = new BooleanQuery.Builder();
    for (JsonNode c : q.get("clauses")) {
      b.add(build(c.get("query"), ix), occur(Json.str(c, "occur", "should")));
    }
    return b.build();
  }

  private static BooleanClause.Occur occur(String s) {
    switch (s) {
      case "must":
        return BooleanClause.Occur.MUST;
      case "must_not":
        return BooleanClause.Occur.MUST_NOT;
      case "filter":
        return BooleanClause.Occur.FILTER;
      default:
        return BooleanClause.Occur.SHOULD;
    }
  }
}
