// Upstream oracle runner for the express/slugify nested parity harness.
//
// JSON Lines on stdio: one request object per line in, exactly one reply object
// per line out, in the same order. Never exits on a failing case — a throw is
// reported as {ok:false}. Everything except replies goes to stderr.
//
// Oracle: npm slugify@1.6.9 (simov/slugify), pinned in package.json.

import readline from "node:readline";
import slugify from "slugify";

// buildOptions maps the language-agnostic case options onto slugify's options
// object. `remove` arrives as a regexp source string so the same pattern can be
// handed to Go's regexp package; it is compiled here with the global flag the
// upstream default uses.
function buildOptions(o) {
	const out = {};
	if (o === null || o === undefined) return out;
	if (typeof o === "string") return o; // string shorthand => {replacement: o}
	if ("replacement" in o) out.replacement = o.replacement;
	if ("lower" in o) out.lower = o.lower;
	if ("strict" in o) out.strict = o.strict;
	if ("trim" in o) out.trim = o.trim;
	if ("locale" in o) out.locale = o.locale;
	if ("remove" in o) out.remove = new RegExp(o.remove, "g");
	return out;
}

function dispatch(fn, args) {
	switch (fn) {
		case "slugify":
			return slugify(args[0]);
		case "slugifyOpts":
			return slugify(args[0], buildOptions(args[1]));
		case "charmap":
			// Transliterate each character of args[0] independently and join with
			// "|" so a single case pinpoints which entry of the charmap differs.
			return [...args[0]].map((ch) => slugify(ch, { trim: false })).join("|");
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
