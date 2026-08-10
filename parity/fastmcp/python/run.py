#!/usr/bin/env python3
"""Upstream (Python FastMCP) parity runner.

Reads one JSON object per line on stdin, writes exactly one JSON object per line
on stdout. Each case drives the *real* upstream FastMCP server built by
server_def.build() over in-memory anyio streams — no sockets, no model provider —
and reports the JSON-RPC answer for the requested MCP method.

Case shapes (identical for the Go runner):

  {"fn": "initialize", "args": [<protocolVersion|null>, "<path>"?]}
  {"fn": "rpc",        "args": [<request>|[<request>,...], "<path>"?]}

  <request> = {"method": "tools/list", "params": {...}}

The emitted value is a list of "outcomes", one per request:

  {"kind": "result", "result": <normalised JSON-RPC result>}
  {"kind": "error",  "code": <JSON-RPC error code>}

Only the *code* is kept for errors: message text is deliberately not compared.
"""

from __future__ import annotations

import json
import sys
import traceback
from typing import Any

import anyio
import mcp.types as types
from mcp.shared.memory import create_client_server_memory_streams
from mcp.shared.message import SessionMessage

from server_def import build

# The protocol version the harness uses for the handshake that precedes every
# non-initialize case.
HANDSHAKE_VERSION = types.LATEST_PROTOCOL_VERSION
CASE_TIMEOUT = 20.0

CLIENT_INFO = {"name": "parity-client", "version": "1.0.0"}
CLIENT_CAPABILITIES: dict[str, Any] = {}


# --------------------------------------------------------------- normalisation

# Volatile or implementation-private keys. `nextCursor` is a pagination cursor,
# `_meta`/`meta` carry framework-private annotations, and `lastModified`/
# `timestamp` are clocks. All are dropped on both sides.
DROP_KEYS = {"nextCursor", "meta", "lastModified", "timestamp"}

# Arrays whose element order carries no meaning.
SORT_STRING_ARRAYS = {"required", "values", "tags"}
SORT_BY_NAME = {
    "tools": "name",
    "prompts": "name",
    "arguments": "name",
    "resources": "uri",
    "resourceTemplates": "uriTemplate",
}


def normalise(value: Any, key: str | None = None) -> Any:
    if isinstance(value, dict):
        out: dict[str, Any] = {}
        for k, v in value.items():
            if k in DROP_KEYS or k.startswith("_"):
                continue
            out[k] = normalise(v, k)
        return out
    if isinstance(value, list):
        items = [normalise(v, None) for v in value]
        if key in SORT_STRING_ARRAYS and all(isinstance(i, str) for i in items):
            items = sorted(items)
        elif key in SORT_BY_NAME:
            field = SORT_BY_NAME[key]
            if all(isinstance(i, dict) for i in items):
                items = sorted(items, key=lambda i: str(i.get(field, "")))
        return items
    return value


def select(value: Any, path: str | None) -> Any:
    """Follow a '/'-separated path. A '#field=value' segment picks the element of
    the current array whose `field` equals `value`.

    A path that cannot be followed yields JSON null rather than an error, so that
    "field absent on one side, present on the other" scores as a value mismatch
    instead of both sides agreeing that they failed.
    """
    if not path:
        return value
    for seg in path.split("/"):
        if seg.startswith("#"):
            field, _, want = seg[1:].partition("=")
            if not isinstance(value, list):
                return None
            for item in value:
                if isinstance(item, dict) and str(item.get(field)) == want:
                    value = item
                    break
            else:
                return None
        elif seg.isdigit():
            if not isinstance(value, list) or int(seg) >= len(value):
                return None
            value = value[int(seg)]
        else:
            if not isinstance(value, dict) or seg not in value:
                return None
            value = value[seg]
    return value


# ------------------------------------------------------------------- transport


class Session:
    """One in-memory client connection to the upstream server."""

    def __init__(self, read, write):
        self._read = read
        self._write = write
        self._next_id = 0

    async def request(self, method: str, params: Any) -> dict[str, Any]:
        self._next_id += 1
        req: dict[str, Any] = {"jsonrpc": "2.0", "id": self._next_id, "method": method}
        if params is not None:
            req["params"] = params
        await self._write.send(
            SessionMessage(message=types.JSONRPCMessage.model_validate(req))
        )
        # Skip server-initiated notifications/requests until our reply arrives.
        while True:
            msg = await self._read.receive()
            if isinstance(msg, Exception):
                raise msg
            root = msg.message.root
            if isinstance(root, types.JSONRPCResponse) and root.id == self._next_id:
                return {"kind": "result", "result": root.result}
            if isinstance(root, types.JSONRPCError) and root.id == self._next_id:
                return {"kind": "error", "code": root.error.code}

    async def notify(self, method: str, params: Any = None) -> None:
        msg: dict[str, Any] = {"jsonrpc": "2.0", "method": method}
        msg["params"] = params if params is not None else {}
        await self._write.send(
            SessionMessage(message=types.JSONRPCMessage.model_validate(msg))
        )


async def with_session(body):
    """Run `body(session)` against a freshly built server on memory streams.

    A fresh server per case keeps every case independent and deterministic, and
    lets the `initialize` cases send their own protocolVersion.
    """
    mcp = build()
    low = mcp._mcp_server
    opts = low.create_initialization_options()

    result: dict[str, Any] = {}
    async with create_client_server_memory_streams() as (client, server):
        client_read, client_write = client
        server_read, server_write = server

        async with anyio.create_task_group() as tg:

            async def serve():
                try:
                    await low.run(
                        server_read,
                        server_write,
                        opts,
                        raise_exceptions=False,
                        stateless=False,
                    )
                except anyio.get_cancelled_exc_class():
                    raise
                except Exception:  # pragma: no cover - server crash is a case loss
                    print(traceback.format_exc(), file=sys.stderr)

            tg.start_soon(serve)
            try:
                with anyio.fail_after(CASE_TIMEOUT):
                    result["value"] = await body(Session(client_read, client_write))
            finally:
                tg.cancel_scope.cancel()
    return result["value"]


# ----------------------------------------------------------------------- cases


async def do_initialize(args: list[Any]) -> Any:
    version = args[0] if args else None
    path = args[1] if len(args) > 1 else None

    async def body(sess: Session):
        params = {
            "protocolVersion": version if version else HANDSHAKE_VERSION,
            "capabilities": CLIENT_CAPABILITIES,
            "clientInfo": CLIENT_INFO,
        }
        out = await sess.request("initialize", params)
        return [project(out, path)]

    return await with_session(body)


async def do_rpc(args: list[Any]) -> Any:
    spec = args[0]
    requests = spec if isinstance(spec, list) else [spec]
    path = args[1] if len(args) > 1 else None

    async def body(sess: Session):
        handshake = await sess.request(
            "initialize",
            {
                "protocolVersion": HANDSHAKE_VERSION,
                "capabilities": CLIENT_CAPABILITIES,
                "clientInfo": CLIENT_INFO,
            },
        )
        if handshake["kind"] != "result":
            raise RuntimeError(f"handshake failed: {handshake}")
        await sess.notify("notifications/initialized")

        outs = []
        for i, req in enumerate(requests):
            out = await sess.request(req["method"], req.get("params"))
            # The path only applies to the final request's result.
            outs.append(project(out, path if i == len(requests) - 1 else None))
        return outs

    return await with_session(body)


def project(outcome: dict[str, Any], path: str | None) -> dict[str, Any]:
    if outcome["kind"] != "result":
        return outcome
    return {"kind": "result", "result": select(normalise(outcome["result"]), path)}


HANDLERS = {"initialize": do_initialize, "rpc": do_rpc}


def main() -> None:
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except Exception as exc:  # unparseable request: nothing to correlate
            print(json.dumps({"id": "", "ok": False, "error": str(exc)}), flush=True)
            continue
        cid = req.get("id", "")
        try:
            fn = req.get("fn")
            if fn not in HANDLERS:
                raise KeyError(f"unknown fn: {fn}")
            value = anyio.run(HANDLERS[fn], req.get("args") or [])
            out = {"id": cid, "ok": True, "value": value}
        except BaseException as exc:  # noqa: BLE001 - never die on a case
            out = {
                "id": cid,
                "ok": False,
                "error": f"{type(exc).__name__}: {exc}",
            }
            print(traceback.format_exc(), file=sys.stderr)
        print(json.dumps(out, sort_keys=True), flush=True)


if __name__ == "__main__":
    main()
