// Upstream oracle runner for the express/nanoid nested parity harness.
//
// JSON Lines on stdio: one request per line in, exactly one reply per line out,
// same order. A throw becomes {ok:false}; the runner never exits early. Logs go
// to stderr.
//
// Oracle: npm nanoid@6.0.1 (ai/nanoid), pinned in package.json.
//
// IMPORTANT: a generated id is random, so no case ever compares generated
// values. Every `fn` below returns a DETERMINISTIC property of generation —
// the alphabet itself, an id's length, whether every character came from the
// expected alphabet, which characters the generator can emit, whether repeated
// calls differ, and whether an invalid size is rejected.

import readline from "node:readline";
import { nanoid, customAlphabet, urlAlphabet } from "nanoid";

// codePoints counts the code points of s. Lengths are compared as code-point
// counts, not UTF-16 units or UTF-8 bytes, so a non-ASCII alphabet means the
// same thing on both sides.
function codePoints(s) {
	return [...s].length;
}

// inSet reports whether every character of id belongs to alphabet.
function inSet(id, alphabet) {
	const set = new Set([...alphabet]);
	return [...id].every((ch) => set.has(ch));
}

// distinctChars returns the sorted, de-duplicated characters of s, which makes
// an observed character set comparable across languages.
function distinctChars(s) {
	return [...new Set([...s])].sort().join("");
}

function dispatch(fn, args) {
	switch (fn) {
		case "urlAlphabet":
			return urlAlphabet;
		case "urlAlphabetLength":
			return codePoints(urlAlphabet);
		case "defaultLength":
			return codePoints(nanoid());
		case "length":
			return codePoints(nanoid(args[0]));
		case "inAlphabet":
			return inSet(nanoid(args[0]), urlAlphabet);
		case "zeroId":
			return nanoid(0);
		case "negative":
			return nanoid(args[0]);
		case "distinct": {
			const [size, count] = args;
			const seen = new Set();
			for (let i = 0; i < count; i++) {
				seen.add(nanoid(size));
			}
			return seen.size === count;
		}
		case "customLength":
			return codePoints(customAlphabet(args[0])(args[1]));
		case "customInAlphabet":
			return inSet(customAlphabet(args[0])(args[1]), args[0]);
		case "customZeroId":
			return customAlphabet(args[0])(0);
		case "customNegative":
			return customAlphabet(args[0])(args[1]);
		case "customCharset": {
			// Which characters can the generator actually emit? Over
			// `samples` ids of `size` characters the observed set converges on
			// the alphabet's distinct characters, so both sides agree
			// deterministically in practice. A generator that silently dropped
			// or duplicated part of the alphabet would show up here.
			const [alphabet, size, samples] = args;
			const generate = customAlphabet(alphabet);
			let seen = "";
			for (let i = 0; i < samples; i++) {
				seen += generate(size);
			}
			return distinctChars(seen);
		}
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
