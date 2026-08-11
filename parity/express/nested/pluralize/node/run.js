// Upstream oracle runner for the express/pluralize nested parity harness.
//
// JSON Lines on stdio: one request per line in, exactly one reply per line out,
// same order. A throw becomes {ok:false}; the runner never exits early. Logs go
// to stderr.
//
// Oracle: npm pluralize@8.0.0 (blakeembrey/pluralize), pinned in package.json.

import readline from "node:readline";
import pluralize from "pluralize";

function dispatch(fn, args) {
	switch (fn) {
		case "plural":
			return pluralize.plural(args[0]);
		case "singular":
			return pluralize.singular(args[0]);
		case "isPlural":
			return pluralize.isPlural(args[0]);
		case "isSingular":
			return pluralize.isSingular(args[0]);
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
