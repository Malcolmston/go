// Upstream oracle runner for the socket.io/client nested parity harness.
//
// Speaks the JSON-Lines stdio contract from ../../../HARNESS.md: one request
// object per line in, exactly one reply object per line out, in order, never
// exiting on a failing case.
//
// The oracle is socket.io-client (the package socket.io/client ports) — and only
// the parts of it that are deterministic and need no server:
//
//   backoff.*   the reconnection schedule, from socket.io-client's own bundled
//               backo2 module (Manager seeds a Backoff from the reconnection
//               options), with Math.random replaced by a fixed sequence
//   endpoint.*  where a connection would go: url() + Manager + the engine.io
//               Socket's websocket Transport.uri(), all with autoConnect off so
//               nothing is dialled
//   options.*   Manager's normalised option defaults
//   wire.*      the packets a Socket builds — CONNECT with auth, EVENT with and
//               without an ack id — encoded with socket.io-parser
//
// Binary event arguments are written {"$bin": "<lowercase hex>"} in the case
// files and become Buffers here; encoded attachment frames come back as
// "bin:<lowercase hex>".
import { createInterface } from "node:readline";
import { Manager, protocol as clientProtocol } from "socket.io-client";
import { Socket as Engine, NodeWebSocket } from "engine.io-client";
import { Encoder } from "socket.io-parser";

// socket.io-client does not re-export these two internals, so they are imported
// by path from the installed package — they are the real upstream modules, not
// reimplementations.
const { url } = await import("./node_modules/socket.io-client/build/esm/url.js");
const { Backoff } = await import("./node_modules/socket.io-client/build/esm/contrib/backo2.js");

const hex = (b) => Buffer.from(b).toString("hex");

// ---------------------------------------------------------------- backoff

// randSequence installs a deterministic Math.random: the case supplies the exact
// draws, cycled if the case asks for more durations than draws.
function withRands(rands, fn) {
  const real = Math.random;
  if (Array.isArray(rands) && rands.length > 0) {
    let i = 0;
    Math.random = () => rands[i++ % rands.length];
  }
  try {
    return fn();
  } finally {
    Math.random = real;
  }
}

function opBackoffDurations(spec) {
  const opts = {};
  if (spec.min !== undefined) opts.min = spec.min;
  if (spec.max !== undefined) opts.max = spec.max;
  if (spec.factor !== undefined) opts.factor = spec.factor;
  if (spec.jitter !== undefined) opts.jitter = spec.jitter;
  const count = spec.count === undefined ? 1 : spec.count;
  return withRands(spec.rands, () => {
    const b = new Backoff(opts);
    const durations = [];
    for (let i = 0; i < count; i++) {
      if (spec.resetAfter !== undefined && i === spec.resetAfter) b.reset();
      durations.push(b.duration());
    }
    return { durations, attempts: b.attempts };
  });
}

// ---------------------------------------------------------------- endpoint

// resolveEndpoint reproduces what socket.io-client does when you call io(uri):
// lookup() parses the uri with url(), hands parsed.source to a Manager, and
// Manager.open() builds `new Engine(this.uri, this.opts)`. Nothing is dialled
// because autoConnect is false; the transport is created but never opened.
function resolveEndpoint(spec) {
  const opts = { autoConnect: false, transports: [NodeWebSocket] };
  if (spec.path !== undefined) opts.path = spec.path;
  const parsed = url(spec.uri, opts.path || "/socket.io");
  const mgr = new Manager(parsed.source, opts);
  const engine = new Engine(mgr.uri, { ...mgr.opts, autoConnect: false, transports: [NodeWebSocket] });
  const transport = engine.createTransport("websocket");
  return { namespace: parsed.path, uri: transport.uri(), enginePath: engine.opts.path };
}

// splitURI separates the query so that the comparison is order-independent:
// upstream appends EIO/transport after the caller's own query parameters, Go's
// url.Values.Encode sorts them, and the difference carries no meaning.
function splitURI(uri) {
  const q = uri.indexOf("?");
  if (q < 0) return { base: uri, query: {} };
  const query = {};
  for (const [k, v] of new URLSearchParams(uri.slice(q + 1))) query[k] = v;
  return { base: uri.slice(0, q), query };
}

function opEndpointResolve(spec) {
  const { namespace, uri } = resolveEndpoint(spec);
  const { base, query } = splitURI(uri);
  return { namespace, base, query };
}

// ---------------------------------------------------------------- options

function opOptionsDefaults(spec) {
  const opts = { autoConnect: false, transports: [NodeWebSocket] };
  for (const k of [
    "path",
    "reconnection",
    "reconnectionAttempts",
    "reconnectionDelay",
    "reconnectionDelayMax",
    "randomizationFactor",
    "timeout",
  ]) {
    if (spec[k] !== undefined) opts[k] = spec[k];
  }
  const mgr = new Manager("http://oracle.invalid", opts);
  // The engine normalises the mount path (adding the trailing slash), so read it
  // from there rather than from the raw option.
  const engine = new Engine(mgr.uri, { ...mgr.opts, autoConnect: false, transports: [NodeWebSocket] });
  const attempts = mgr.reconnectionAttempts();
  return {
    path: engine.opts.path,
    reconnection: mgr.reconnection(),
    // Infinity has no JSON form; unlimited is reported as null on both sides.
    reconnectionAttempts: Number.isFinite(attempts) ? attempts : null,
    reconnectionDelay: mgr.reconnectionDelay(),
    reconnectionDelayMax: mgr.reconnectionDelayMax(),
    randomizationFactor: mgr.randomizationFactor(),
    timeout: mgr.timeout(),
  };
}

// ---------------------------------------------------------------- wire

// reviveArgs turns {"$bin":"<hex>"} markers into Buffers, recursively.
function reviveArgs(v) {
  if (Array.isArray(v)) return v.map(reviveArgs);
  if (v && typeof v === "object") {
    if (typeof v.$bin === "string" && Object.keys(v).length === 1) return Buffer.from(v.$bin, "hex");
    const out = {};
    for (const k of Object.keys(v)) out[k] = reviveArgs(v[k]);
    return out;
  }
  return v;
}

// encodeFrames renders an encoded packet the way both runners report it: the
// text frame first, then each binary attachment as "bin:<hex>".
function encodeFrames(packet) {
  return new Encoder().encode(packet).map((f) => (typeof f === "string" ? f : "bin:" + hex(f)));
}

// newSocket returns a Socket for nsp that is deliberately never connected, so
// emits land in sendBuffer and _sendConnectPacket can be intercepted.
function newSocket(nsp, opts = {}) {
  const mgr = new Manager("http://oracle.invalid", { autoConnect: false, transports: [NodeWebSocket] });
  return mgr.socket(nsp === undefined ? "/" : nsp, opts);
}

function opWireConnect(spec) {
  const sock = newSocket(spec.namespace, spec.auth === undefined ? {} : { auth: reviveArgs(spec.auth) });
  const captured = [];
  sock.io._packet = (p) => captured.push(p);
  sock._sendConnectPacket(spec.auth === undefined ? undefined : reviveArgs(spec.auth));
  if (captured.length !== 1) throw new Error("expected exactly one CONNECT packet, got " + captured.length);
  return { frames: encodeFrames(captured[0]) };
}

function opWireEmit(spec) {
  const sock = newSocket(spec.namespace);
  const args = reviveArgs(spec.args === undefined ? [] : spec.args);
  if (spec.ackId === undefined) {
    sock.emit(spec.event, ...args);
  } else {
    // `ids` is the ack-id allocator; seeding it pins the id this emit claims.
    sock.ids = spec.ackId;
    sock.emit(spec.event, ...args, () => {});
  }
  if (sock.sendBuffer.length !== 1) {
    throw new Error("expected exactly one buffered packet, got " + sock.sendBuffer.length);
  }
  const packet = sock.sendBuffer[0];
  // Socket.packet() stamps the namespace on its way out; buffered packets have
  // not been through it yet.
  packet.nsp = sock.nsp;
  delete packet.options;
  return { frames: encodeFrames(packet), ackId: packet.id === undefined ? null : packet.id };
}

// opWireAckIDs pins the allocator itself: n acknowledged emits in a row.
function opWireAckIDs(spec) {
  const sock = newSocket(spec.namespace);
  const ids = [];
  for (let i = 0; i < spec.count; i++) {
    sock.emit("ev", () => {});
    ids.push(sock.sendBuffer[sock.sendBuffer.length - 1].id);
  }
  return { ids };
}

function opProtocol() {
  return { protocol: clientProtocol };
}

function dispatch(req) {
  const a0 = req.args && req.args.length > 0 ? req.args[0] : {};
  switch (req.fn) {
    case "backoff.durations":
      return opBackoffDurations(a0);
    case "endpoint.resolve":
      return opEndpointResolve(a0);
    case "options.defaults":
      return opOptionsDefaults(a0);
    case "wire.connect":
      return opWireConnect(a0);
    case "wire.emit":
      return opWireEmit(a0);
    case "wire.ackIds":
      return opWireAckIDs(a0);
    case "client.protocol":
      return opProtocol();
    default:
      throw new Error("unknown fn: " + req.fn);
  }
}

// ---------------------------------------------------------------- stdio loop

const rl = createInterface({ input: process.stdin, crlfDelay: Infinity });

rl.on("line", (line) => {
  const text = line.trim();
  if (text === "") return;
  let req;
  try {
    req = JSON.parse(text);
  } catch (err) {
    process.stdout.write(JSON.stringify({ id: "?", ok: false, error: "bad request: " + err.message }) + "\n");
    return;
  }
  let reply;
  try {
    reply = { id: req.id, ok: true, value: dispatch(req) };
  } catch (err) {
    reply = { id: req.id, ok: false, error: String((err && err.message) || err) };
  }
  process.stdout.write(JSON.stringify(reply) + "\n");
});
