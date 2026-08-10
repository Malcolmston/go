#!/usr/bin/env node
// Upstream oracle runner for the socket.io parity harness.
//
// Speaks the JSON-Lines contract described in parity/HARNESS.md: one JSON object
// per line in, one JSON object per line out, in order. Never exits on a failing
// case — every handler is wrapped so a throw becomes {"ok":false,"error":...}.
//
// Groups it serves:
//   wire.*      socket.io-parser Encoder/Decoder (exact wire bytes)
//   eio.*       engine.io-parser packet/payload framing
//   scenario.*  end-to-end behaviour transcript (own server + own client)
//   serve.*     start a server for cross-implementation interop, return its URL
//   drive.*     connect a client to a given URL, return the client transcript
import readline from "node:readline";
import http from "node:http";
import { Buffer } from "node:buffer";
import { Server } from "socket.io";
import { io as ioClient } from "socket.io-client";
import { Encoder, Decoder } from "socket.io-parser";
import {
  encodePacket,
  decodePacket,
  encodePayload,
  decodePayload,
} from "engine.io-parser";

const ACK_WAIT = 400; // ms — long enough for a local ack, short enough to fail fast
const STEP_WAIT = 300; // ms — settle window for fan-out (broadcast) scenarios
const SCENARIO_TIMEOUT = 8000; // ms — hard cap on any single scenario

// ---------------------------------------------------------------- helpers

const EIO_TYPES = ["open", "close", "ping", "pong", "message", "upgrade", "noop"];

const hex = (b) => Buffer.from(b).toString("hex");

const isBin = (v) =>
  Buffer.isBuffer(v) || v instanceof Uint8Array || v instanceof ArrayBuffer;

// toNative turns {"$bin":"<hex>"} markers from the case JSON into Buffers.
function toNative(v) {
  if (Array.isArray(v)) return v.map(toNative);
  if (v && typeof v === "object") {
    if (typeof v.$bin === "string" && Object.keys(v).length === 1) {
      return Buffer.from(v.$bin, "hex");
    }
    const out = {};
    for (const k of Object.keys(v)) out[k] = toNative(v[k]);
    return out;
  }
  return v;
}

// fromNative turns Buffers back into {"$bin":"<hex>"} markers for the report.
function fromNative(v) {
  if (isBin(v)) return { $bin: hex(v) };
  if (Array.isArray(v)) return v.map(fromNative);
  if (v && typeof v === "object") {
    const out = {};
    for (const k of Object.keys(v).sort()) out[k] = fromNative(v[k]);
    return out;
  }
  return v;
}

const j = (v) => JSON.stringify(fromNative(v));

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function withTimeout(promise, ms, tag) {
  let t;
  return Promise.race([
    promise.finally(() => clearTimeout(t)),
    new Promise((_, rej) => {
      t = setTimeout(() => rej(new Error("timeout: " + tag)), ms);
    }),
  ]);
}

// A transcript collects ordered, normalised observation lines. Volatile socket
// ids are replaced by <S1>, <S2>, ... in first-seen order.
function newTranscript() {
  const lines = [];
  const ids = new Map();
  return {
    lines,
    sid(id) {
      if (id == null) return "<nil>";
      if (!ids.has(id)) ids.set(id, `<S${ids.size + 1}>`);
      return ids.get(id);
    },
    push(...parts) {
      lines.push(parts.join(" "));
    },
  };
}

// ---------------------------------------------------------------- wire group

function wireEncode(spec) {
  const pkt = { type: spec.type, nsp: spec.nsp === undefined ? "/" : spec.nsp };
  if (spec.data !== undefined && spec.data !== null) pkt.data = toNative(spec.data);
  if (spec.id !== undefined && spec.id !== null) pkt.id = spec.id;
  const frames = new Encoder().encode(pkt);
  return {
    frames: frames.map((f) => (typeof f === "string" ? f : { $bin: hex(f) })),
  };
}

function feed(spec) {
  const dec = new Decoder();
  const packets = [];
  dec.on("decoded", (p) => packets.push(p));
  try {
    for (const f of spec.frames) {
      dec.add(typeof f === "string" ? f : Buffer.from(f.$bin, "hex"));
    }
  } finally {
    // leave reconstructor state readable by the caller before destroy
  }
  return { dec, packets };
}

function wireDecode(spec) {
  const { dec, packets } = feed(spec);
  const pending = dec.reconstructor != null;
  dec.destroy();
  return {
    packets: packets.map((p) => ({
      type: p.type,
      nsp: p.nsp,
      id: p.id === undefined ? null : p.id,
      data: p.data === undefined ? null : fromNative(p.data),
    })),
    pending,
  };
}

function wireDecodeAttachments(spec) {
  const { dec, packets } = feed(spec);
  dec.destroy();
  return {
    attachments: packets.map((p) => (p.attachments === undefined ? null : p.attachments)),
  };
}

// ---------------------------------------------------------------- eio group

function eioEncodePacket(spec) {
  const pkt = { type: EIO_TYPES[spec.type] };
  if (spec.binary !== undefined && spec.binary !== null) {
    pkt.data = Buffer.from(spec.binary, "hex");
  } else if (spec.data !== undefined && spec.data !== null) {
    pkt.data = spec.data;
  }
  let out;
  encodePacket(pkt, false, (v) => {
    out = v;
  });
  if (out === undefined) throw new Error("encodePacket did not call back synchronously");
  return { encoded: out };
}

function normEioPacket(p) {
  if (p.type === "error") throw new Error("parser error: " + p.data);
  const t = EIO_TYPES.indexOf(p.type);
  if (t < 0) throw new Error("unknown engine.io packet type: " + p.type);
  if (isBin(p.data)) return { type: t, data: "", binary: hex(p.data) };
  return { type: t, data: p.data === undefined ? "" : p.data, binary: null };
}

function eioDecodePacket(spec) {
  return { packet: normEioPacket(decodePacket(spec.encoded, "nodebuffer")) };
}

function eioEncodePayload(spec) {
  const packets = spec.packets.map((s) => {
    const pkt = { type: EIO_TYPES[s.type] };
    if (s.binary !== undefined && s.binary !== null) pkt.data = Buffer.from(s.binary, "hex");
    else if (s.data !== undefined && s.data !== null) pkt.data = s.data;
    return pkt;
  });
  let out;
  encodePayload(packets, (v) => {
    out = v;
  });
  if (out === undefined) throw new Error("encodePayload did not call back synchronously");
  return { encoded: out };
}

function eioDecodePayload(spec) {
  const packets = decodePayload(spec.encoded, "nodebuffer");
  return { packets: packets.map(normEioPacket) };
}

// ---------------------------------------------------------------- e2e plumbing

const live = { io: null, http: null, transcript: null, url: null };

async function startServer(setup) {
  const httpServer = http.createServer();
  const io = new Server(httpServer, {
    // Long heartbeats keep the heartbeat out of the transcript entirely.
    pingInterval: 25000,
    pingTimeout: 20000,
    cors: { origin: true },
  });
  const t = newTranscript();
  setup(io, t);
  await new Promise((res) => httpServer.listen(0, "127.0.0.1", res));
  const url = `http://127.0.0.1:${httpServer.address().port}`;
  return { io, httpServer, url, t };
}

async function stopServer(io, httpServer) {
  await new Promise((res) => {
    let done = false;
    const fin = () => {
      if (!done) {
        done = true;
        res();
      }
    };
    io.close(fin);
    setTimeout(fin, 1500);
  });
  try {
    httpServer.close();
  } catch {}
}

// emitAck emits with a raw ack callback and a manual timeout, so the FULL list
// of ack arguments is observable. (socket.io-client's own emitWithAck resolves
// with a single value, which would hide multi-argument acks.)
function emitAck(target, ev, args, ms) {
  return new Promise((res) => {
    let done = false;
    const timer = setTimeout(() => {
      if (!done) {
        done = true;
        res(null);
      }
    }, ms);
    target.emit(ev, ...args, (...ackArgs) => {
      if (!done) {
        done = true;
        clearTimeout(timer);
        res(ackArgs);
      }
    });
  });
}

function connect(url, opts = {}) {
  const nsp = opts.namespace || "/";
  const sock = ioClient(url + nsp, {
    transports: ["websocket"],
    reconnection: false,
    forceNew: true,
    auth: opts.auth,
    timeout: 3000,
  });
  return sock;
}

// waitConnect resolves with {ok:true} on connect, {ok:false,message} on
// connect_error. Never hangs: bounded by ACK_WAIT*5.
function waitConnect(sock) {
  return withTimeout(
    new Promise((res) => {
      sock.once("connect", () => res({ ok: true }));
      sock.once("connect_error", (e) => res({ ok: false, message: e.message }));
    }),
    2000,
    "connect",
  );
}

// ---------------------------------------------------------------- scenarios

const scenarios = {
  // ---- emit + argument marshalling
  async "emit-scalar-args"() {
    const { io, httpServer, url, t } = await startServer((io, t) => {
      io.on("connection", (s) => {
        t.push("srv:connection", t.sid(s.id));
        s.on("e", (...a) => t.push("srv:e", j(a)));
        s.on("done", () => t.push("srv:done"));
      });
    });
    const c = connect(url);
    t.push("cli:connect", JSON.stringify((await waitConnect(c)).ok));
    c.emit("e", "s", 1, 1.5, true, false, null);
    c.emit("e");
    c.emit("e", { b: 1, a: 2 }, [1, [2, [3]]]);
    c.emit("e", "", 0, -1, 1e21);
    c.emit("done");
    await withTimeout(
      new Promise((res) => {
        const iv = setInterval(() => {
          if (t.lines.some((l) => l === "srv:done")) {
            clearInterval(iv);
            res();
          }
        }, 10);
      }),
      2000,
      "done",
    );
    c.close();
    await stopServer(io, httpServer);
    return t.lines;
  },

  // ---- acks: client -> server
  async "ack-values"() {
    return ackScenario("sum", (s, t) => {
      s.on("sum", (a, b, cb) => {
        t.push("srv:sum", j([a, b]));
        cb(a + b);
      });
    }, ["sum", 1, 2]);
  },

  async "ack-multi-values"() {
    return ackScenario("multi", (s, t) => {
      s.on("multi", (cb) => {
        t.push("srv:multi");
        cb(1, "two", { k: 1 }, null);
      });
    }, ["multi"]);
  },

  async "ack-empty-call"() {
    return ackScenario("empty", (s, t) => {
      s.on("empty", (cb) => {
        t.push("srv:empty");
        cb();
      });
    }, ["empty"]);
  },

  // KNOWN PORT DIVERGENCE: upstream never acks an event with no handler.
  async "ack-no-handler"() {
    return ackScenario("ghost", () => {}, ["ghost", 1]);
  },

  // KNOWN PORT DIVERGENCE: a handler that ignores its callback must not ack.
  async "ack-handler-never-calls"() {
    return ackScenario("silent", (s, t) => {
      s.on("silent", () => {
        t.push("srv:silent");
      });
    }, ["silent"]);
  },

  async "ack-binary"() {
    return ackScenario("bin", (s, t) => {
      s.on("bin", (buf, cb) => {
        t.push("srv:bin", j([buf]));
        cb(Buffer.from([9, 8, 7]));
      });
    }, ["bin", { $bin: "0001ff" }]);
  },

  // ---- acks: server -> client
  async "server-ack"() {
    return serverAckScenario((c, t) => {
      c.on("q", (arg, cb) => {
        t.push("cli:q", j([arg]));
        cb("y");
      });
    });
  },

  // KNOWN PORT DIVERGENCE (client side): the Go client also auto-acks.
  async "server-ack-no-handler"() {
    return serverAckScenario(() => {});
  },

  // ---- catch-all
  async onany() {
    const { io, httpServer, url, t } = await startServer((io, t) => {
      io.on("connection", (s) => {
        // Drop the ack callback: it is not a wire argument and the Go port's
        // OnAny signature has no equivalent for it.
        s.onAny((ev, ...a) => t.push("srv:any", ev, j(a.filter((x) => typeof x !== "function"))));
        s.on("b", (cb) => cb("acked"));
        s.on("z", () => t.push("srv:z"));
      });
    });
    const c = connect(url);
    await waitConnect(c);
    c.emit("a", 1);
    const reply = await emitAck(c, "b", [], ACK_WAIT);
    t.push("cli:ack-b", reply === null ? "<timeout>" : j(reply));
    c.emit("z", { k: [1, 2] });
    await sleep(STEP_WAIT);
    c.close();
    await stopServer(io, httpServer);
    return t.lines;
  },

  // ---- binary
  async "binary-server-to-client"() {
    const { io, httpServer, url, t } = await startServer((io, t) => {
      io.on("connection", (s) => {
        s.on("ready", () =>
          s.emit("blob", Buffer.from([0, 1, 254, 255]), "tail", {
            nested: Buffer.from("hi"),
          }),
        );
      });
    });
    const c = connect(url);
    await waitConnect(c);
    c.on("blob", (...a) => t.push("cli:blob", j(a)));
    c.emit("ready");
    await sleep(STEP_WAIT);
    c.close();
    await stopServer(io, httpServer);
    return t.lines;
  },

  async "binary-client-to-server"() {
    const { io, httpServer, url, t } = await startServer((io, t) => {
      io.on("connection", (s) => {
        s.on("blob", (...a) => t.push("srv:blob", j(a)));
      });
    });
    const c = connect(url);
    await waitConnect(c);
    c.emit("blob", Buffer.from([0, 1, 254, 255]), [Buffer.from("ab"), 7]);
    await sleep(STEP_WAIT);
    c.close();
    await stopServer(io, httpServer);
    return t.lines;
  },

  // ---- namespaces + middleware
  async "namespace-connect"() {
    const { io, httpServer, url, t } = await startServer((io, t) => {
      io.of("/admin").on("connection", (s) => {
        t.push("srv:connection", s.nsp.name, t.sid(s.id));
      });
      io.on("connection", () => t.push("srv:root-connection"));
    });
    const c = connect(url, { namespace: "/admin" });
    const r = await waitConnect(c);
    t.push("cli:connect", JSON.stringify(r.ok));
    await sleep(STEP_WAIT);
    c.close();
    await stopServer(io, httpServer);
    return t.lines;
  },

  async "namespace-middleware-accept"() {
    const { io, httpServer, url, t } = await startServer((io, t) => {
      const ns = io.of("/admin");
      ns.use((s, next) => {
        t.push("srv:mw1");
        next();
      });
      ns.use((s, next) => {
        t.push("srv:mw2");
        next();
      });
      ns.on("connection", () => t.push("srv:connection"));
    });
    const c = connect(url, { namespace: "/admin" });
    t.push("cli:connect", JSON.stringify((await waitConnect(c)).ok));
    await sleep(STEP_WAIT);
    c.close();
    await stopServer(io, httpServer);
    return t.lines;
  },

  async "namespace-middleware-reject"() {
    const { io, httpServer, url, t } = await startServer((io, t) => {
      const ns = io.of("/admin");
      ns.use((s, next) => {
        t.push("srv:mw1");
        next(new Error("nope"));
      });
      ns.use((s, next) => {
        t.push("srv:mw2-should-not-run");
        next();
      });
      ns.on("connection", () => t.push("srv:connection-should-not-run"));
    });
    const c = connect(url, { namespace: "/admin" });
    const r = await waitConnect(c);
    t.push("cli:connect", JSON.stringify(r.ok), r.ok ? "" : r.message);
    await sleep(STEP_WAIT);
    c.close();
    await stopServer(io, httpServer);
    return t.lines;
  },

  // A middleware that never calls next() must leave the connection hanging.
  async "namespace-middleware-silent"() {
    const { io, httpServer, url, t } = await startServer((io, t) => {
      const ns = io.of("/admin");
      ns.use(() => {
        t.push("srv:mw-no-next");
      });
      ns.on("connection", () => t.push("srv:connection-should-not-run"));
    });
    const c = connect(url, { namespace: "/admin" });
    const r = await waitConnect(c).catch(() => ({ ok: false, message: "<no reply>" }));
    t.push("cli:connect", JSON.stringify(r.ok), r.ok ? "" : r.message);
    c.close();
    await stopServer(io, httpServer);
    return t.lines;
  },

  async "namespace-invalid"() {
    const { io, httpServer, url, t } = await startServer((io) => {
      io.on("connection", () => {});
    });
    const c = connect(url, { namespace: "/ghost" });
    const r = await waitConnect(c);
    t.push("cli:connect", JSON.stringify(r.ok), r.ok ? "" : r.message);
    c.close();
    await stopServer(io, httpServer);
    return t.lines;
  },

  async "handshake-auth"() {
    const { io, httpServer, url, t } = await startServer((io, t) => {
      io.use((s, next) => {
        t.push("srv:auth", j(s.handshake.auth));
        next();
      });
      io.on("connection", () => t.push("srv:connection"));
    });
    const c = connect(url, { auth: { token: "abc", n: 1 } });
    t.push("cli:connect", JSON.stringify((await waitConnect(c)).ok));
    c.close();
    await stopServer(io, httpServer);
    return t.lines;
  },

  // ---- rooms + broadcast
  async "rooms-self-room"() {
    const { io, httpServer, url, t } = await startServer((io, t) => {
      const show = (s) =>
        j([...s.rooms].map((r) => (r === s.id ? "<self>" : r)).sort());
      io.on("connection", (s) => {
        t.push("srv:rooms-initial", show(s));
        s.join("a");
        s.join("b");
        t.push("srv:rooms-joined", show(s));
        s.leave("a");
        t.push("srv:rooms-left", show(s));
      });
    });
    const c = connect(url);
    await waitConnect(c);
    await sleep(STEP_WAIT);
    c.close();
    await stopServer(io, httpServer);
    return t.lines;
  },

  "rooms-socket-to-excludes-sender": () =>
    broadcastScenario((s, t) => {
      s.on("shout", (msg) => {
        t.push("srv:shout-from", t.sid(s.id));
        s.to("r").emit("heard", msg);
      });
    }),

  "rooms-namespace-to-includes-sender": () =>
    broadcastScenario((s, t, io) => {
      s.on("shout", (msg) => {
        t.push("srv:shout-from", t.sid(s.id));
        io.to("r").emit("heard", msg);
      });
    }),

  "rooms-except": () =>
    broadcastScenario((s, t, io) => {
      s.on("shout", (msg) => {
        t.push("srv:shout-from", t.sid(s.id));
        io.except("r").emit("heard", msg);
      });
    }),

  "rooms-broadcast-all": () =>
    broadcastScenario((s, t, io) => {
      s.on("shout", (msg) => {
        t.push("srv:shout-from", t.sid(s.id));
        io.emit("heard", msg);
      });
    }),

  "rooms-socket-broadcast": () =>
    broadcastScenario((s, t) => {
      s.on("shout", (msg) => {
        t.push("srv:shout-from", t.sid(s.id));
        s.broadcast.emit("heard", msg);
      });
    }),

  async "rooms-fetch-sockets"() {
    const { io, httpServer, url, t } = await startServer((io, t) => {
      io.on("connection", (s) => {
        s.join("r");
      });
    });
    const cs = [];
    for (let i = 0; i < 3; i++) {
      const c = connect(url);
      await waitConnect(c);
      cs.push(c);
    }
    await sleep(STEP_WAIT);
    t.push("srv:all", String((await io.fetchSockets()).length));
    t.push("srv:in-room", String((await io.in("r").fetchSockets()).length));
    t.push("srv:in-missing", String((await io.in("nope").fetchSockets()).length));
    for (const c of cs) c.close();
    await stopServer(io, httpServer);
    return t.lines;
  },

  async "rooms-sockets-join-leave"() {
    const { io, httpServer, url, t } = await startServer(() => {});
    const cs = [];
    for (let i = 0; i < 2; i++) {
      const c = connect(url);
      await waitConnect(c);
      cs.push(c);
    }
    await sleep(STEP_WAIT);
    io.socketsJoin("bulk");
    await sleep(50);
    t.push("srv:bulk", String((await io.in("bulk").fetchSockets()).length));
    io.socketsLeave("bulk");
    await sleep(50);
    t.push("srv:bulk-after", String((await io.in("bulk").fetchSockets()).length));
    for (const c of cs) c.close();
    await stopServer(io, httpServer);
    return t.lines;
  },

  // ---- disconnects
  async "disconnect-client-initiated"() {
    const { io, httpServer, url, t } = await startServer((io, t) => {
      io.on("connection", (s) => {
        s.on("disconnecting", (r) => t.push("srv:disconnecting", r));
        s.on("disconnect", (r) => t.push("srv:disconnect", r));
      });
    });
    const c = connect(url);
    await waitConnect(c);
    c.disconnect();
    await sleep(STEP_WAIT);
    await stopServer(io, httpServer);
    return t.lines;
  },

  async "disconnect-server-namespace"() {
    return serverDisconnectScenario(false);
  },

  async "disconnect-server-transport"() {
    return serverDisconnectScenario(true);
  },

  async "disconnect-namespace-then-emit"() {
    const { io, httpServer, url, t } = await startServer((io, t) => {
      io.on("connection", (s) => {
        s.on("go", () => {
          s.disconnect(false);
          try {
            s.emit("after");
            t.push("srv:emit-after-ok");
          } catch (e) {
            t.push("srv:emit-after-threw");
          }
        });
      });
    });
    const c = connect(url);
    await waitConnect(c);
    c.on("after", () => t.push("cli:after"));
    c.on("disconnect", (r) => t.push("cli:disconnect", r));
    c.emit("go");
    await sleep(STEP_WAIT);
    c.close();
    await stopServer(io, httpServer);
    return t.lines;
  },

  // ---- reserved event names
  async "reserved-event-server-emit"() {
    const { io, httpServer, url, t } = await startServer((io, t) => {
      io.on("connection", (s) => {
        for (const name of ["connect", "disconnect", "connect_error"]) {
          try {
            s.emit(name, 1);
            t.push("srv:emit-ok", name);
          } catch (e) {
            t.push("srv:emit-threw", name);
          }
        }
      });
    });
    const c = connect(url);
    await waitConnect(c);
    await sleep(STEP_WAIT);
    c.close();
    await stopServer(io, httpServer);
    return t.lines;
  },

  async "reserved-event-client-emit"() {
    const { io, httpServer, url, t } = await startServer((io) => {
      io.on("connection", () => {});
    });
    const c = connect(url);
    await waitConnect(c);
    for (const name of ["connect", "disconnect", "connect_error"]) {
      try {
        c.emit(name, 1);
        t.push("cli:emit-ok", name);
      } catch (e) {
        t.push("cli:emit-threw", name);
      }
    }
    c.close();
    await stopServer(io, httpServer);
    return t.lines;
  },

  // ---- socket-scoped state and handler removal
  async "handler-off"() {
    const { io, httpServer, url, t } = await startServer((io, t) => {
      io.on("connection", (s) => {
        const h = () => t.push("srv:hit");
        s.on("x", h);
        s.on("kill", () => {
          s.off("x", h);
          t.push("srv:killed");
        });
      });
    });
    const c = connect(url);
    await waitConnect(c);
    c.emit("x");
    await sleep(100);
    c.emit("kill");
    await sleep(100);
    c.emit("x");
    await sleep(STEP_WAIT);
    c.close();
    await stopServer(io, httpServer);
    return t.lines;
  },

  async "socket-data"() {
    const { io, httpServer, url, t } = await startServer((io, t) => {
      io.use((s, next) => {
        s.data.who = "mw";
        next();
      });
      io.on("connection", (s) => {
        t.push("srv:data", j(s.data));
        s.data.n = 1;
        t.push("srv:data2", j(s.data));
      });
    });
    const c = connect(url);
    await waitConnect(c);
    await sleep(STEP_WAIT);
    c.close();
    await stopServer(io, httpServer);
    return t.lines;
  },
};

async function ackScenario(name, setup, emitArgs) {
  const { io, httpServer, url, t } = await startServer((io, t) => {
    io.on("connection", (s) => setup(s, t));
  });
  const c = connect(url);
  await waitConnect(c);
  const [ev, ...rest] = emitArgs;
  const reply = await emitAck(c, ev, rest.map(toNative), ACK_WAIT);
  await sleep(50);
  t.push(reply === null ? "cli:ack-timeout" : "cli:ack " + j(reply));
  c.close();
  await stopServer(io, httpServer);
  return t.lines;
}

async function serverAckScenario(clientSetup) {
  const done = { fn: null };
  const { io, httpServer, url, t } = await startServer((io, t) => {
    // Gated on a client "ready" event: the Go client cannot register handlers
    // before its Dial returns, so a server that emits straight out of the
    // connection handler would race the client's listener on that side only.
    io.on("connection", (s) => {
      s.on("ready", async () => {
        const v = await emitAck(s, "q", ["x"], ACK_WAIT);
        t.push(v === null ? "srv:ack-timeout" : "srv:ack " + j(v));
        if (done.fn) done.fn();
      });
    });
  });
  const finished = new Promise((res) => {
    done.fn = res;
  });
  const c = connect(url);
  await waitConnect(c);
  clientSetup(c, t);
  c.emit("ready");
  await withTimeout(finished, 2000, "server-ack").catch(() => t.push("srv:never-finished"));
  await sleep(50);
  c.close();
  await stopServer(io, httpServer);
  return t.lines;
}

async function serverDisconnectScenario(closeTransport) {
  const { io, httpServer, url, t } = await startServer((io, t) => {
    io.on("connection", (s) => {
      s.on("disconnect", (r) => t.push("srv:disconnect", r));
      s.on("go", () => s.disconnect(closeTransport));
    });
  });
  const c = connect(url);
  await waitConnect(c);
  c.on("disconnect", (r) => t.push("cli:disconnect", r));
  c.emit("go");
  await sleep(STEP_WAIT);
  c.close();
  await stopServer(io, httpServer);
  return t.lines;
}

// broadcastScenario connects three clients in order; clients 0 and 1 join room
// "r"; client 0 emits "shout". Receiver lines are sorted before reporting
// because fan-out order across sockets is not specified.
async function broadcastScenario(onShout) {
  const { io, httpServer, url, t } = await startServer((io, t) => {
    let n = 0;
    io.on("connection", (s) => {
      const idx = n++;
      t.sid(s.id);
      if (idx < 2) s.join("r");
      onShout(s, t, io);
    });
  });
  const cs = [];
  for (let i = 0; i < 3; i++) {
    const c = connect(url);
    await waitConnect(c);
    const idx = i;
    c.on("heard", (msg) => t.push("cli:heard", "c" + idx, j([msg])));
    cs.push(c);
  }
  await sleep(STEP_WAIT);
  cs[0].emit("shout", "hey");
  await sleep(STEP_WAIT);
  for (const c of cs) c.close();
  await stopServer(io, httpServer);
  const head = t.lines.filter((l) => !l.startsWith("cli:heard"));
  const heard = t.lines.filter((l) => l.startsWith("cli:heard")).sort();
  return [...head, ...heard];
}

// ---------------------------------------------------------------- interop

// The interop script is deliberately restricted to what BOTH clients can do, so
// the client transcript is comparable across implementations.
const interopServers = {
  basic(io, t) {
    io.on("connection", (s) => {
      t.push("srv:connection", t.sid(s.id));
      s.on("echo", (...args) => {
        const cb = typeof args[args.length - 1] === "function" ? args.pop() : null;
        t.push("srv:echo", j(args));
        if (cb) cb("echoed", args.length);
      });
      s.on("bin", (...args) => {
        const cb = typeof args[args.length - 1] === "function" ? args.pop() : null;
        t.push("srv:bin", j(args));
        if (cb) cb(Buffer.from([1, 2, 3]));
      });
      s.on("ask", () => {
        emitAck(s, "question", ["q1"], ACK_WAIT).then((v) =>
          t.push(v === null ? "srv:answer-timeout" : "srv:answer " + j(v)),
        );
      });
      s.on("push-me", () => s.emit("pushed", "p", 2, { k: true }));
      s.on("disconnect", (r) => t.push("srv:disconnect", r));
    });
  },
};

async function driveInterop(url, t) {
  const c = connect(url);
  const r = await waitConnect(c);
  t.push("cli:connect", JSON.stringify(r.ok));
  if (!r.ok) {
    t.push("cli:connect-error", r.message);
    c.close();
    return t.lines;
  }
  c.on("pushed", (...a) => t.push("cli:pushed", j(a)));
  c.on("question", (arg, cb) => {
    t.push("cli:question", j([arg]));
    cb("a1");
  });

  c.emit("echo", "s", 1, true, null, { b: 1 });
  await sleep(100);

  const ack = await emitAck(c, "echo", ["with-ack"], ACK_WAIT);
  t.push(ack === null ? "cli:ack-timeout" : "cli:ack " + j(ack));

  const binAck = await emitAck(c, "bin", [Buffer.from([0, 255])], ACK_WAIT);
  t.push(binAck === null ? "cli:bin-ack-timeout" : "cli:bin-ack " + j(binAck));

  c.emit("push-me");
  await sleep(150);
  c.emit("ask");
  await sleep(ACK_WAIT + 150);
  c.close();
  await sleep(100);
  return t.lines;
}

// ---------------------------------------------------------------- dispatch

async function handle(req) {
  const fn = req.fn;
  const a0 = (req.args || [])[0];
  switch (fn) {
    case "wire.encode":
      return wireEncode(a0);
    case "wire.decode":
      return wireDecode(a0);
    case "wire.decodeAttachments":
      return wireDecodeAttachments(a0);
    case "eio.encodePacket":
      return eioEncodePacket(a0);
    case "eio.decodePacket":
      return eioDecodePacket(a0);
    case "eio.encodePayload":
      return eioEncodePayload(a0);
    case "eio.decodePayload":
      return eioDecodePayload(a0);
    case "meta.versions":
      return { protocol: 5, engineio: 4 };
    case "serve.close": {
      if (!live.io) throw new Error("no live server");
      const t = live.transcript;
      await stopServer(live.io, live.http);
      live.io = live.http = live.transcript = live.url = null;
      return { transcript: t.lines };
    }
    default:
      break;
  }
  if (fn.startsWith("scenario.")) {
    const name = fn.slice("scenario.".length);
    const sc = scenarios[name];
    if (!sc) throw new Error("unknown scenario: " + name);
    return { transcript: await withTimeout(Promise.resolve(sc()), SCENARIO_TIMEOUT, fn) };
  }
  if (fn.startsWith("serve.")) {
    const name = fn.slice("serve.".length);
    const setup = interopServers[name];
    if (!setup) throw new Error("unknown interop server: " + name);
    if (live.io) throw new Error("server already running");
    const { io, httpServer, url, t } = await startServer(setup);
    live.io = io;
    live.http = httpServer;
    live.transcript = t;
    live.url = url;
    return { url };
  }
  if (fn.startsWith("drive.")) {
    const name = fn.slice("drive.".length);
    if (!interopServers[name]) throw new Error("unknown interop script: " + name);
    const t = newTranscript();
    return {
      transcript: await withTimeout(driveInterop(a0, t), SCENARIO_TIMEOUT, fn).catch((e) => {
        t.push("cli:fatal", String(e && e.message));
        return t.lines;
      }),
    };
  }
  throw new Error("unknown fn: " + fn);
}

const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
for await (const line of rl) {
  const s = line.trim();
  if (!s) continue;
  let req;
  try {
    req = JSON.parse(s);
  } catch (e) {
    process.stdout.write(JSON.stringify({ id: "?", ok: false, error: "bad request: " + e.message }) + "\n");
    continue;
  }
  let out;
  try {
    out = { id: req.id, ok: true, value: await handle(req) };
  } catch (e) {
    out = { id: req.id, ok: false, error: String((e && e.message) || e) };
  }
  process.stdout.write(JSON.stringify(out) + "\n");
}
// Any lingering server must not keep the process alive.
if (live.io) await stopServer(live.io, live.http);
process.exit(0);
