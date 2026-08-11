// Upstream oracle runner for the express/kebabcase nested parity harness.
//
// JSON Lines on stdio: one request per line in, exactly one reply per line out,
// same order. A throw becomes {ok:false}; the runner never exits early. Logs go
// to stderr.
//
// Oracle: npm change-case@5.4.4 (blakeembrey/change-case), the `kebabCase`
// export, pinned in package.json. See COVERAGE.md for why change-case rather
// than npm `kebab-case` is the oracle for this port.

import readline from "node:readline";
import { kebabCase } from "change-case";

function dispatch(fn, args) {
	switch (fn) {
		case "kebabCase":
			return kebabCase(args[0]);
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
