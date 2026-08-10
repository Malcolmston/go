"""The shared parity server definition, built with upstream Python FastMCP.

The Go runner in ../go/run.go builds the *same* server (same tool names, same
argument names and types, same resources, same prompts) so that every MCP
protocol answer can be compared field by field.

Nothing here touches the network or a model provider: the server is driven over
in-memory anyio streams by run.py.
"""

from __future__ import annotations

from typing import Any

from pydantic import BaseModel, Field

from fastmcp import FastMCP

SERVER_NAME = "parity-server"
SERVER_VERSION = "1.0.0"
INSTRUCTIONS = "A fixed server used to compare MCP wire behaviour across ports."

# A deterministic 8-byte "PNG" used by the binary resource. Both runners embed
# the identical bytes so the base64 blob must match exactly.
LOGO_BYTES = bytes([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A])


class Stats(BaseModel):
    """Summary statistics for a list of numbers."""

    count: int = Field(description="how many values were summarised")
    total: float = Field(description="the sum of the values")
    mean: float = Field(description="the arithmetic mean")


def build() -> FastMCP:
    mcp = FastMCP(name=SERVER_NAME, version=SERVER_VERSION, instructions=INSTRUCTIONS)

    # ----------------------------------------------------------------- tools

    @mcp.tool(name="add", description="Add two integers.")
    def add(
        a: int = Field(description="the first addend"),
        b: int = Field(description="the second addend"),
    ) -> int:
        return a + b

    @mcp.tool(name="greet", description="Greet someone by name.")
    def greet(
        name: str = Field(description="who to greet"),
        greeting: str = Field(default="Hello", description="the greeting word"),
    ) -> str:
        return f"{greeting}, {name}!"

    @mcp.tool(name="search", description="Search with several defaulted options.")
    def search(
        query: str = Field(description="the search query"),
        limit: int = Field(default=10, description="maximum hits to return"),
        verbose: bool = Field(default=False, description="include debug detail"),
        tags: list[str] = Field(default=[], description="restrict to these tags"),
    ) -> str:
        # Render the bool the way JSON does, so the compared text is language-neutral.
        flag = "true" if verbose else "false"
        return f"{query}|{limit}|{flag}|{','.join(tags)}"

    @mcp.tool(name="echo_map", description="Echo a free-form JSON object.")
    def echo_map(
        payload: dict[str, Any] = Field(description="any JSON object"),
    ) -> str:
        import json as _json

        return _json.dumps(payload, sort_keys=True, separators=(",", ":"))

    @mcp.tool(name="nickname", description="Build a label with an optional nickname.")
    def nickname(
        name: str = Field(description="the person's name"),
        nickname: str | None = Field(default=None, description="an optional nickname"),
    ) -> str:
        return name if nickname is None else f"{name} ({nickname})"

    @mcp.tool(name="stats", description="Summarise a list of numbers.")
    def stats(
        values: list[float] = Field(description="the numbers to summarise"),
    ) -> Stats:
        total = float(sum(values))
        count = len(values)
        return Stats(count=count, total=total, mean=(total / count) if count else 0.0)

    @mcp.tool(name="fail", description="Always fail, to exercise tool errors.")
    def fail(
        reason: str = Field(description="why this call should fail"),
    ) -> str:
        raise ValueError(f"deliberate failure: {reason}")

    # ------------------------------------------------------------- resources

    @mcp.resource(
        "res://greeting",
        name="greeting",
        description="A fixed greeting.",
        mime_type="text/plain",
    )
    def greeting_resource() -> str:
        return "hello from the parity server"

    @mcp.resource(
        "res://data.json",
        name="data",
        description="A fixed JSON document.",
        mime_type="application/json",
    )
    def data_resource() -> str:
        return '{"answer":42,"kind":"json"}'

    @mcp.resource(
        "res://logo.png",
        name="logo",
        description="A fixed binary blob.",
        mime_type="image/png",
    )
    def logo_resource() -> bytes:
        return LOGO_BYTES

    @mcp.resource(
        "res://user/{id}",
        name="user",
        description="A templated user record.",
        mime_type="text/plain",
    )
    def user_resource(id: str) -> str:
        return f"user:{id}"

    # --------------------------------------------------------------- prompts

    @mcp.prompt(name="summarize", description="Summarise a block of text.")
    def summarize(
        text: str = Field(description="the text to summarise"),
        style: str = Field(default="concise", description="the summary style"),
    ) -> str:
        return f"Summarise the following in a {style} style:\n{text}"

    @mcp.prompt(name="review", description="Review a snippet of code.")
    def review(
        code: str = Field(description="the code to review"),
    ) -> str:
        return f"Review this code:\n{code}"

    return mcp
