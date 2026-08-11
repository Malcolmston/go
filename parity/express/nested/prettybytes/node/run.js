// Upstream oracle runner for the express/prettybytes nested parity harness.
//
// JSON Lines on stdio: one request per line in, exactly one reply per line out,
// same order. A throw becomes {ok:false}; the runner never exits early. Logs go
// to stderr.
//
// Oracle: npm pretty-bytes@7.1.1 (sindresorhus/pretty-bytes), pinned in
// package.json.

import readline from "node:readline";
import prettyBytes from "pretty-bytes";

// buildOptions passes through only the option keys the Go port models. The
// locale-dependent path is exercised with an explicit locale so the answer does
// not depend on the machine's default ICU locale.
function buildOptions(o) {
	const out = {};
	if (o === null || o === undefined) return out;
	if ("bits" in o) out.bits = o.bits;
	if ("binary" in o) out.binary = o.binary;
	if ("signed" in o) out.signed = o.signed;
	if ("minimumFractionDigits" in o) out.minimumFractionDigits = o.minimumFractionDigits;
	if ("maximumFractionDigits" in o) out.maximumFractionDigits = o.maximumFractionDigits;
	if ("locale" in o) out.locale = o.locale;
	return out;
}

// numberArg turns the JSON representation of the input back into a number.
// JSON has no literal for the non-finite values, so cases pass them as strings.
function numberArg(value) {
	if (typeof value === "string") {
		switch (value) {
			case "NaN":
				return Number.NaN;
			case "Infinity":
				return Number.POSITIVE_INFINITY;
			case "-Infinity":
				return Number.NEGATIVE_INFINITY;
			default:
				return Number(value);
		}
	}

	return value;
}

function dispatch(fn, args) {
	switch (fn) {
		case "prettyBytes":
			return prettyBytes(numberArg(args[0]));
		case "prettyBytesOpts":
			return prettyBytes(numberArg(args[0]), buildOptions(args[1]));
		default:
			throw new Error(`unknown fn: ${fn}`);
	}
}

const rl = readline.createInterface({ input: process.stdin, terminal: false });
rl.on("line", (line) => {
	if (line.trim() === "") return;
	let req;
	try {
		req = JSON.parse(line);
	} catch (error) {
		process.stderr.write(`bad request line: ${error.message}\n`);
		return;
	}
	let reply;
	try {
		reply = { id: req.id, ok: true, value: dispatch(req.fn, req.args || []) };
	} catch (error) {
		reply = { id: req.id, ok: false, error: String(error && error.message ? error.message : error) };
	}
	process.stdout.write(`${JSON.stringify(reply)}\n`);
});
