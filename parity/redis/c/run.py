#!/usr/bin/env python3
"""Upstream (oracle) parity runner for the real Redis server.

Speaks the parity JSON-Lines contract: one JSON object per line on stdin, exactly
one JSON object per line on stdout, never exiting on a failing case.

The oracle is a *real* `redis-server` (the binary that ships with redis-cli),
started by this script on a random free port with a throwaway temp dir and
persistence disabled (`--save ''` `--appendonly no`), so nothing is ever written
to the user's machine and an already-running Redis on 6379 is never touched.
Commands are spoken to it over a plain TCP socket in RESP2 — the same wire the
`redis-cli` shipped alongside it uses — because raw RESP preserves the reply
*type* (simple string vs bulk string vs nil vs integer vs array vs error) that
`redis-cli`'s human-readable output flattens away.

Each case is a SCRIPT of commands against a fresh, empty database (new
connection + FLUSHALL per case, so no MULTI/WATCH state can leak between cases).
The emitted value is

    {"steps": [<normalised reply per step>], "dump": [<canonical key dump>]}

Reply -> JSON mapping (identical on both sides, see COVERAGE.md):

    RESP integer        -> JSON number
    RESP bulk string    -> JSON string (UTF-8; non-UTF-8 replies are an error)
    RESP nil            -> JSON null
    RESP simple string  -> {"status": "OK"}
    RESP array          -> JSON array (nil array -> null)
    RESP error          -> {"error": true}   (message text is never compared)
"""

import atexit
import json
import os
import shutil
import socket
import subprocess
import sys
import tempfile
import time

STDERR = sys.stderr


def log(*a):
    print(*a, file=STDERR, flush=True)


# --------------------------------------------------------------- reply objects

class Status(str):
    """A RESP simple string ("+OK")."""


class RespError(Exception):
    pass


# ------------------------------------------------------------------ the server

class Server:
    def __init__(self):
        self.dir = tempfile.mkdtemp(prefix="parity-redis-")
        self.port = self._free_port()
        exe = shutil.which("redis-server")
        if exe is None:
            raise RuntimeError("redis-server not found in PATH")
        self.proc = subprocess.Popen(
            [
                exe,
                "--port", str(self.port),
                "--bind", "127.0.0.1",
                "--dir", self.dir,
                "--save", "",
                "--appendonly", "no",
                "--daemonize", "no",
                "--appendfsync", "no",
                "--maxmemory-policy", "noeviction",
                "--enable-debug-command", "no",
            ],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        atexit.register(self.stop)
        self._wait_ready()

    @staticmethod
    def _free_port():
        s = socket.socket()
        s.bind(("127.0.0.1", 0))
        port = s.getsockname()[1]
        s.close()
        return port

    def _wait_ready(self):
        deadline = time.time() + 20.0
        while time.time() < deadline:
            if self.proc.poll() is not None:
                raise RuntimeError("redis-server exited during startup")
            try:
                c = Conn(self.port)
                if c.call(["PING"]) is not None:
                    c.close()
                    return
                c.close()
            except OSError:
                time.sleep(0.05)
        raise RuntimeError("redis-server did not become ready")

    def stop(self):
        p = getattr(self, "proc", None)
        if p is not None and p.poll() is None:
            try:
                c = Conn(self.port)
                c.sock.sendall(b"*1\r\n$8\r\nSHUTDOWN\r\n$6\r\nNOSAVE\r\n")
                c.close()
            except OSError:
                pass
            try:
                p.wait(timeout=5)
            except subprocess.TimeoutExpired:
                p.kill()
                p.wait(timeout=5)
        shutil.rmtree(getattr(self, "dir", ""), ignore_errors=True)


# ---------------------------------------------------------------- RESP2 client

class Conn:
    def __init__(self, port):
        self.sock = socket.create_connection(("127.0.0.1", port), timeout=20.0)
        self.sock.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
        self.buf = b""

    def close(self):
        try:
            self.sock.close()
        except OSError:
            pass

    def call(self, args):
        out = bytearray(b"*%d\r\n" % len(args))
        for a in args:
            b = a.encode("utf-8")
            out += b"$%d\r\n" % len(b)
            out += b
            out += b"\r\n"
        self.sock.sendall(bytes(out))
        return self._read()

    def _line(self):
        while b"\r\n" not in self.buf:
            chunk = self.sock.recv(65536)
            if not chunk:
                raise OSError("server closed the connection")
            self.buf += chunk
        line, self.buf = self.buf.split(b"\r\n", 1)
        return line

    def _exact(self, n):
        while len(self.buf) < n + 2:
            chunk = self.sock.recv(65536)
            if not chunk:
                raise OSError("server closed the connection")
            self.buf += chunk
        data, self.buf = self.buf[:n], self.buf[n + 2:]
        return data

    def _read(self):
        line = self._line()
        tag, rest = line[:1], line[1:]
        if tag == b"+":
            return Status(rest.decode("utf-8"))
        if tag == b"-":
            raise RespError(rest.decode("utf-8", "replace"))
        if tag == b":":
            return int(rest)
        if tag == b"$":
            n = int(rest)
            if n < 0:
                return None
            raw = self._exact(n)
            try:
                return raw.decode("utf-8")
            except UnicodeDecodeError:
                raise RespError("reply is not valid UTF-8")
        if tag == b"*":
            n = int(rest)
            if n < 0:
                return None
            return [self._read() for _ in range(n)]
        raise RespError("unexpected RESP type %r" % tag)


# ------------------------------------------------------- shared normalisation
# Kept byte-for-byte equivalent to the Go runner's normalise/sort rules.

# Commands whose reply order Redis does not define: sorted on both sides.
UNORDERED = {
    "KEYS", "SMEMBERS", "SINTER", "SUNION", "SDIFF", "SRANDMEMBER", "SPOP",
    "HKEYS", "HVALS",
}
# Commands replying with a flat field/value list: sorted as pairs on both sides.
PAIRED = {"HGETALL", "CONFIG"}


def to_json(v):
    if v is None:
        return None
    if isinstance(v, Status):
        return {"status": str(v)}
    if isinstance(v, bool):
        return v
    if isinstance(v, int):
        return v
    if isinstance(v, str):
        return v
    if isinstance(v, list):
        return [to_json(x) for x in v]
    raise RespError("cannot encode reply %r" % (v,))


def sort_key(x):
    return json.dumps(x, sort_keys=True)


def normalise(cmd, val, round_to=0):
    j = to_json(val)
    cmd = cmd.upper()
    if isinstance(j, list):
        if cmd in UNORDERED:
            j = sorted(j, key=sort_key)
        elif cmd in PAIRED and len(j) % 2 == 0:
            pairs = [j[i:i + 2] for i in range(0, len(j), 2)]
            pairs.sort(key=sort_key)
            j = [x for p in pairs for x in p]
        elif cmd in ("SCAN", "HSCAN", "SSCAN", "ZSCAN") and len(j) == 2 and isinstance(j[1], list):
            j = [j[0], sorted(j[1], key=sort_key)]
    if round_to and isinstance(j, int) and not isinstance(j, bool) and j > 0:
        j = ((j + round_to - 1) // round_to) * round_to
    return j


# --------------------------------------------------------------- the key dump

def dump(conn):
    """Canonical dump of the whole database: every key sorted, with type,
    TTL presence and value. Only commands that the Go port can also dispatch
    are used, so the dump itself never biases the comparison."""
    try:
        keys = conn.call(["KEYS", "*"])
    except (RespError, OSError):
        return [{"error": True}]
    keys = sorted(str(k) for k in (keys or []))
    out = []
    for k in keys:
        ent = {"key": k}
        try:
            ent["type"] = normalise("TYPE", conn.call(["TYPE", k]))
        except RespError:
            ent["type"] = {"error": True}
        try:
            t = conn.call(["TTL", k])
            ent["ttl"] = {-1: "none", -2: "gone"}.get(t, "set")
        except RespError:
            ent["ttl"] = {"error": True}
        kind = ent["type"].get("status") if isinstance(ent["type"], dict) else None
        try:
            if kind == "string":
                ent["value"] = normalise("GET", conn.call(["GET", k]))
            elif kind == "list":
                ent["value"] = normalise("LRANGE", conn.call(["LRANGE", k, "0", "-1"]))
            elif kind == "hash":
                ent["value"] = normalise("HGETALL", conn.call(["HGETALL", k]))
            elif kind == "set":
                ent["value"] = normalise("SMEMBERS", conn.call(["SMEMBERS", k]))
            elif kind == "zset":
                ent["value"] = normalise(
                    "ZRANGE", conn.call(["ZRANGE", k, "0", "-1", "WITHSCORES"]))
            else:
                ent["value"] = None
        except RespError:
            ent["value"] = {"error": True}
        out.append(ent)
    return out


# ------------------------------------------------------------------- the loop

def run_script(server, steps):
    conn = Conn(server.port)
    try:
        conn.call(["FLUSHALL"])
        results = []
        for st in steps:
            op = st.get("op", "cmd")
            if op == "sleep":
                time.sleep(float(st.get("ms", 0)) / 1000.0)
                results.append(None)
                continue
            if op != "cmd":
                results.append({"error": True})
                continue
            args = [str(a) for a in st.get("args", [])]
            if not args:
                results.append({"error": True})
                continue
            try:
                rep = conn.call(args)
                results.append(normalise(args[0], rep, int(st.get("round", 0) or 0)))
            except RespError:
                results.append({"error": True})
        return {"steps": results, "dump": dump(conn)}
    finally:
        conn.close()


def command_surface(server):
    """The upstream command surface, enumerated mechanically from the running
    server with COMMAND COUNT + COMMAND LIST (the same data `redis-cli COMMAND
    DOCS` reports). Container subcommands ("acl|cat") are folded into their
    parent, so the list holds top-level command names only and its length equals
    COMMAND COUNT."""
    conn = Conn(server.port)
    try:
        count = conn.call(["COMMAND", "COUNT"])
        names = conn.call(["COMMAND", "LIST"]) or []
        top = sorted({str(n).upper() for n in names if "|" not in str(n)})
        return {"commandCount": count, "commands": top}
    finally:
        conn.close()


def main():
    server = Server()
    log("redis-server listening on 127.0.0.1:%d (dir %s)" % (server.port, server.dir))
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        rid = None
        try:
            req = json.loads(line)
            rid = req.get("id")
            args = req.get("args") or []
            if req.get("fn") == "commands":
                val = command_surface(server)
                print(json.dumps({"id": rid, "ok": True, "value": val}), flush=True)
                continue
            if req.get("fn") != "script" or not args or not isinstance(args[0], list):
                raise ValueError("expected fn=script with one array argument")
            val = run_script(server, args[0])
            print(json.dumps({"id": rid, "ok": True, "value": val}, sort_keys=True), flush=True)
        except Exception as e:  # never exit on a failing case
            print(json.dumps({"id": rid, "ok": False, "error": "%s: %s" % (type(e).__name__, e)}),
                  flush=True)
    server.stop()


if __name__ == "__main__":
    main()
