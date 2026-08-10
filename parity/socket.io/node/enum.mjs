#!/usr/bin/env node
// Mechanical API inventory of the installed upstream packages, used to build
// COVERAGE.md. Walks the real prototypes (never the README) and prints one
// "Class.member" per line, sorted.
//
//   node enum.mjs            # everything
//   node enum.mjs Server     # one class
import http from "node:http";
import { Server, Namespace, Socket } from "socket.io";
import { io as ioClient, Manager, Socket as ClientSocket } from "socket.io-client";
// BroadcastOperator is not exported; reach it through a real instance.
import * as parser from "socket.io-parser";
import * as eio from "engine.io-parser";

// Members inherited from EventEmitter / Emitter are listed separately: they are
// generic emitter plumbing rather than Socket.IO API.
const EMITTER = new Set([
  "on", "off", "once", "emit", "addListener", "removeListener", "removeAllListeners",
  "listeners", "listenerCount", "eventNames", "setMaxListeners", "getMaxListeners",
  "prependListener", "prependOnceListener", "rawListeners", "constructor",
  "hasListeners", "emitReserved", "emitUntyped", "_emit",
]);

function walk(label, obj, { includeEmitter = false } = {}) {
  const out = new Set();
  let o = obj;
  while (o && o !== Object.prototype && o !== Function.prototype) {
    for (const k of Reflect.ownKeys(o)) {
      if (typeof k !== "string") continue;
      if (k.startsWith("_")) continue; // internal by convention
      if (!includeEmitter && EMITTER.has(k)) continue;
      out.add(k);
    }
    o = Object.getPrototypeOf(o);
  }
  return [...out].sort().map((k) => `${label}.${k}`);
}

// Instance-only accessors (getters defined on the instance, plus own fields).
function instanceMembers(label, inst) {
  const out = new Set();
  for (const k of Object.keys(inst)) {
    if (k.startsWith("_")) continue;
    out.add(k);
  }
  return [...out].sort().map((k) => `${label}.${k}(own)`);
}

const httpServer = http.createServer();
const io = new Server(httpServer);
const nsp = io.of("/x");
const bop = io.to("room");
const csock = ioClient("http://127.0.0.1:1/", { autoConnect: false });

const groups = {
  Server: [
    ...walk("Server", Server.prototype),
    ...instanceMembers("Server", io),
  ],
  Namespace: [
    ...walk("Namespace", Namespace.prototype),
    ...instanceMembers("Namespace", nsp),
  ],
  Socket: [
    ...walk("Socket", Socket.prototype),
  ],
  BroadcastOperator: [
    ...walk("BroadcastOperator", Object.getPrototypeOf(bop)),
  ],
  ClientSocket: [
    ...walk("ClientSocket", ClientSocket.prototype),
  ],
  Manager: [...walk("Manager", Manager.prototype)],
  "socket.io-parser": Object.keys(parser).sort().map((k) => `parser.${k}`),
  "engine.io-parser": Object.keys(eio).sort().map((k) => `eio.${k}`),
  "socket.io (module)": Object.keys(await import("socket.io")).sort().map((k) => `socket.io.${k}`),
  "socket.io-client (module)": Object.keys(await import("socket.io-client")).sort().map((k) => `socket.io-client.${k}`),
};

const only = process.argv[2];
for (const [label, members] of Object.entries(groups)) {
  if (only && label !== only) continue;
  console.log(`## ${label} (${members.length})`);
  for (const m of members) console.log(m);
  console.log();
}

csock.close();
io.close();
httpServer.close();
process.exit(0);
