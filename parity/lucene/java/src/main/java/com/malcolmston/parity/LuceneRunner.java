package com.malcolmston.parity;

import com.fasterxml.jackson.databind.JsonNode;
import org.apache.lucene.analysis.en.EnglishAnalyzer;
import org.apache.lucene.queryparser.classic.QueryParser;
import org.apache.lucene.search.Query;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.io.PrintStream;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;

/**
 * JSON-Lines parity runner for Apache Lucene. Reads one case per line on stdin,
 * writes exactly one answer per line on stdout, and never exits on a failing
 * case: every throwable becomes {"ok":false}.
 *
 * <p>Usage: java -jar runner-shaded.jar &lt;path to fixtures/corpus.json&gt;
 */
public final class LuceneRunner {
  public static void main(String[] args) throws Exception {
    PrintStream out = new PrintStream(System.out, true, StandardCharsets.UTF_8);
    JsonNode fixture = Json.M.readTree(Files.readString(Path.of(args[0])));
    Indexes.build(fixture);

    BufferedReader in = new BufferedReader(new InputStreamReader(System.in, StandardCharsets.UTF_8));
    String line;
    while ((line = in.readLine()) != null) {
      if (line.isBlank()) {
        continue;
      }
      JsonNode c = Json.M.readTree(line);
      String id = c.get("id").asText();
      try {
        out.println(Json.M.writeValueAsString(Json.ok(id, dispatch(c))));
      } catch (Throwable t) {
        out.println(Json.M.writeValueAsString(Json.fail(id, t)));
      }
      out.flush();
    }
  }

  private static Object dispatch(JsonNode c) throws Exception {
    String fn = c.get("fn").asText();
    JsonNode args = c.get("args");
    JsonNode a0 = args.size() > 0 ? args.get(0) : null;

    switch (fn) {
      case "analyze":
        return Analyzers.tokens(Analyzers.of(a0.get("analyzer").asText()), a0.get("text").asText());
      case "analyzePositions":
        return Analyzers.positions(
            Analyzers.of(a0.get("analyzer").asText()), a0.get("text").asText());
      case "defaultStopWords": {
        List<String> w = new ArrayList<>();
        for (Object o : EnglishAnalyzer.ENGLISH_STOP_WORDS_SET) {
          w.add(new String((char[]) o));
        }
        Collections.sort(w);
        return w;
      }
      case "ngrams":
        return Utils.ngrams(a0.asText(), args.get(1).asInt());
      case "edgeNgrams":
        return Utils.edgeNgrams(a0.asText(), args.get(1).asInt(), args.get(2).asInt());
      case "shingles": {
        List<String> toks = new ArrayList<>();
        for (JsonNode t : a0) {
          toks.add(t.asText());
        }
        return Utils.shingles(toks, args.get(1).asInt());
      }
      case "wildcardMatch":
        return Utils.wildcardMatch(a0.asText(), args.get(1).asText());
      case "withinEdits":
        return Utils.withinEdits(a0.asText(), args.get(1).asText(), args.get(2).asInt());
      default:
        return indexed(fn, a0);
    }
  }

  private static Object indexed(String fn, JsonNode a) throws Exception {
    Indexes ix = Indexes.get(a.get("index").asText());
    int topN = Json.num(a, "topN", 10);
    switch (fn) {
      case "numDocs":
        return ix.reader.numDocs();
      case "fields":
        return Indexes.fields();
      case "terms":
        return Searches.terms(ix, Json.str(a, "field", "text"));
      case "termCount":
        return Searches.terms(ix, Json.str(a, "field", "text")).size();
      case "docFreq":
        return Searches.docFreq(ix, Json.str(a, "field", "text"), Json.str(a, "term", ""));
      case "totalTermFreq":
        return Searches.totalTermFreq(ix, Json.str(a, "field", "text"), Json.str(a, "term", ""));
      case "search":
        return Searches.ranked(ix, Queries.build(a.get("query"), ix), topN);
      case "searchSet":
        return Searches.set(ix, Queries.build(a.get("query"), ix), topN);
      case "parse":
        return Searches.set(ix, parse(ix, a.get("query").asText()), topN);
      case "parseRanked":
        return Searches.ranked(ix, parse(ix, a.get("query").asText()), topN);
      default:
        throw new IllegalArgumentException("unknown fn: " + fn);
    }
  }

  private static Query parse(Indexes ix, String q) throws Exception {
    return new QueryParser("text", ix.analyzer).parse(q);
  }
}
