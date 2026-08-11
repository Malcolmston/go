// Upstream oracle runner for the express/deburr nested parity harness.
//
// JSON Lines on stdio: one request per line in, exactly one reply per line out,
// same order. A throw becomes {ok:false}; the runner never exits early. Logs go
// to stderr.
//
// Oracle: npm lodash@4.17.21, the `deburr` method. See COVERAGE.md for why the
// maintained lodash package rather than the frozen lodash.deburr@4.1.0
// standalone is the oracle for this port.

import readline from "node:readline";
import deburr from "lodash/deburr.js";

function dispatch(fn, args) {
	switch (fn) {
		case "deburr":
			return deburr(args[0]);
		case "perChar":
			// Deburr each code point independently and join with "|" so a single
			// case pinpoints which table entry differs.
			return [...args[0]].map((ch) => deburr(ch)).join("|");
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
