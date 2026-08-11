// Upstream oracle runner for the express/filesize nested parity harness.
//
// JSON Lines on stdio: one request per line in, exactly one reply per line out,
// same order. A throw becomes {ok:false}; the runner never exits early. Logs go
// to stderr.
//
// Oracle: npm filesize@11.0.22 (avoidwork/filesize), pinned in package.json.

import readline from "node:readline";
import { filesize } from "filesize";

// buildOptions passes through only the option keys the Go port models, so a
// case can never accidentally exercise an upstream-only knob on one side alone.
function buildOptions(o) {
	const out = {};
	if (o === null || o === undefined) return out;
	if ("base" in o) out.base = o.base;
	if ("round" in o) out.round = o.round;
	if ("standard" in o) out.standard = o.standard;
	return out;
}

function dispatch(fn, args) {
	switch (fn) {
		case "filesize":
			return filesize(args[0]);
		case "filesizeOpts":
			return filesize(args[0], buildOptions(args[1]));
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
