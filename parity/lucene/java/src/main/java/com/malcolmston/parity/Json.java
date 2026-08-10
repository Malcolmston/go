package com.malcolmston.parity;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;

import java.util.LinkedHashMap;
import java.util.Map;

/** Tiny JSON helpers shared by the runner classes. */
final class Json {
  static final ObjectMapper M = new ObjectMapper();

  private Json() {}

  static String str(JsonNode n, String field, String dflt) {
    JsonNode v = n.get(field);
    return (v == null || v.isNull()) ? dflt : v.asText();
  }

  static int num(JsonNode n, String field, int dflt) {
    JsonNode v = n.get(field);
    return (v == null || v.isNull()) ? dflt : v.asInt();
  }

  static double dbl(JsonNode n, String field, double dflt) {
    JsonNode v = n.get(field);
    return (v == null || v.isNull()) ? dflt : v.asDouble();
  }

  static boolean bool(JsonNode n, String field, boolean dflt) {
    JsonNode v = n.get(field);
    return (v == null || v.isNull()) ? dflt : v.asBoolean();
  }

  /** Empty string means "unbounded" in the port's range API; upstream wants null. */
  static String orNull(String s) {
    return (s == null || s.isEmpty()) ? null : s;
  }

  static Map<String, Object> hits(int total, Object ids) {
    Map<String, Object> m = new LinkedHashMap<>();
    m.put("total", total);
    m.put("ids", ids);
    return m;
  }

  static ObjectNode ok(String id, Object value) {
    ObjectNode o = M.createObjectNode();
    o.put("id", id);
    o.put("ok", true);
    o.set("value", M.valueToTree(value));
    return o;
  }

  static ObjectNode fail(String id, Throwable t) {
    ObjectNode o = M.createObjectNode();
    o.put("id", id);
    o.put("ok", false);
    String msg = t.getMessage();
    o.put("error", t.getClass().getSimpleName() + (msg == null ? "" : ": " + msg));
    return o;
  }
}
