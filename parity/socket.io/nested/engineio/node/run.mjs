// Upstream oracle runner for the socket.io/engineio nested parity harness.
//
// Speaks the JSON-Lines stdio contract from ../../../HARNESS.md: one request
// object per line in, exactly one reply object per line out, in order. Never
// exits on a failing case — a throw becomes {"ok":false,"error":...}.
//
// The oracle is engine.io-parser (the package socket.io/engineio ports), plus
// engine.io itself for the protocol revision.
//
// Conventions shared with the Go runner (go/run.go):
//
//   - a packet is {"type": <engine.io type name>, "data": <string>,
//     "binary": <lowercase hex string or null>}. Binary payloads are hex
//     because JSON has no byte type.
//   - upstream signals a parse failure by returning its ERROR_PACKET
//     ({type:"error",data:"parser error"}) rather than throwing; the runner
//     turns that into ok:false so it lines up with the Go codec's error return.
//   - encodePacket is driven with supportsBinary=false (the polling form: "b" +
//     standard base64), which is what engineio.Packet.Encode produces.
import { createInterface } from "node:readline";
import {
  encodePacket,
  decodePacket,
  encodePayload,
  decodePayload,
  protocol as parserProtocol,
} from "engine.io-parser";
import { protocol as engineProtocol } from "engine.io";

const SEP = String.fromCharCode(0x1e);

const TYPES = ["open", "close", "ping", "pong", "message", "upgrade", "noop"];

const isBuf = (v) => typeof Buffer !== "undefined" && Buffer.isBuffer(v);
const hex = (b) => Buffer.from(b).toString("hex");

// ---------------------------------------------------------------- marshalling

// specToPacket turns a case's packet spec into the shape engine.io-parser wants.
function specToPacket(spec) {
  if (spec === null || typeof spec !== "object") throw new Error("packet spec must be an object");
  if (!TYPES.includes(spec.type)) throw new Error("unknown engine.io packet type: " + spec.type);
  const pkt = { type: spec.type };
  if (spec.binary !== undefined && spec.binary !== null) {
    pkt.data = Buffer.from(spec.binary, "hex");
  } else if (spec.data !== undefined && spec.data !== null) {
    pkt.data = spec.data;
  }
  return pkt;
}

// normPacket renders a decoded upstream packet in the comparison shape, and
// converts upstream's in-band ERROR_PACKET into a thrown parse failure.
function normPacket(p) {
  if (p.type === "error") throw new Error("parser error: " + p.data);
  if (!TYPES.includes(p.type)) throw new Error("unknown engine.io packet type: " + p.type);
  if (isBuf(p.data)) return { type: p.type, data: "", binary: hex(p.data) };
  return { type: p.type, data: p.data === undefined ? "" : p.data, binary: null };
}

// sync unwraps engine.io-parser's callback style, which is synchronous for
// every input this harness feeds it.
function sync(fn) {
  let out;
  let called = false;
  fn((v) => {
    out = v;
    called = true;
  });
  if (!called) throw new Error("engine.io-parser did not call back synchronously");
  return out;
}

// ---------------------------------------------------------------- operations

function opEncodePacket(spec) {
  const supportsBinary = spec.supportsBinary === true;
  const out = sync((cb) => encodePacket(specToPacket(spec), supportsBinary, cb));
  if (isBuf(out)) return { kind: "binary", encoded: hex(out) };
  if (typeof out !== "string") throw new Error("encodePacket produced " + typeof out);
  return { kind: "string", encoded: out };
}

function opDecodePacket(spec) {
  return { packet: normPacket(decodePacket(spec.encoded, "nodebuffer")) };
}

// opDecodeBinaryFrame is the websocket binary-frame path: decodePacket accepts
// the raw Buffer of the frame and yields a binary MESSAGE.
function opDecodeBinaryFrame(spec) {
  return { packet: normPacket(decodePacket(Buffer.from(spec.binary, "hex"), "nodebuffer")) };
}

function opEncodePayload(spec) {
  if (!Array.isArray(spec.packets)) throw new Error("packets must be an array");
  const packets = spec.packets.map(specToPacket);
  return { encoded: sync((cb) => encodePayload(packets, cb)) };
}

function opDecodePayload(spec) {
  const packets = decodePayload(spec.encoded, "nodebuffer");
  return { packets: packets.map(normPacket) };
}

// opRoundTrip encodes a batch as a polling payload and decodes it again, which
// is the only property a batching format really has to hold.
function opRoundTrip(spec) {
  const encoded = sync((cb) => encodePayload(spec.packets.map(specToPacket), cb));
  return { encoded, packets: decodePayload(encoded, "nodebuffer").map(normPacket) };
}

function opProtocol() {
  return { parser: parserProtocol, engine: engineProtocol, separator: hex(SEP) };
}

function dispatch(req) {
  const a0 = req.args && req.args.length > 0 ? req.args[0] : {};
  switch (req.fn) {
    case "eio.encodePacket":
      return opEncodePacket(a0);
    case "eio.decodePacket":
      return opDecodePacket(a0);
    case "eio.decodeBinaryFrame":
      return opDecodeBinaryFrame(a0);
    case "eio.encodePayload":
      return opEncodePayload(a0);
    case "eio.decodePayload":
      return opDecodePayload(a0);
    case "eio.roundTrip":
      return opRoundTrip(a0);
    case "eio.protocol":
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
