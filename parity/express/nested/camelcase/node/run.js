// Upstream oracle runner for the express/camelcase nested parity harness.
//
// JSON Lines on stdio: one request per line in, exactly one reply per line out,
// same order, flushed each time. A throw becomes {ok:false}; the runner never
// exits early. Logs go to stderr.
//
// Oracle: npm camelcase@9.0.0 (sindresorhus/camelcase), pinned in package.json.

import readline from "node:readline";
import camelCase from "camelcase";

function dispatch(fn, args) {
	switch (fn) {
		case "camelCase":
			return camelCase(args[0]);
		case "pascalCase":
			return camelCase(args[0], { pascalCase: true });
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
