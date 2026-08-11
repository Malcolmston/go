// Upstream oracle runner for parity/passport/nested/pkce.
//
// Speaks JSON Lines on stdio: one request object per line in, exactly one reply
// object per line out, in the same order. The oracle is pkce-challenge@6.0.0,
// the npm implementation of RFC 7636 Proof Key for Code Exchange.
//
// Conventions, mirrored by go/run.go:
//   * Challenge methods travel in their RFC 7636 spelling ("S256" / "plain").
//   * `verify` answers a verdict, never an error: a method no implementation
//     recognises cannot verify a credential, so it is reported as false on both
//     sides. `challenge` does propagate the error, because there the answer is a
//     derived value and there is none.
//   * `generatedProperties` is the only case family that touches randomness. It
//     compares properties of a freshly generated verifier — its length, and that
//     it round-trips through its own challenge — never the verifier itself.
//
// Never exits on a failing case: the throw is reported as {ok:false,error}.
import readline from 'node:readline';
import pkceChallenge, { generateChallenge, verifyChallenge } from 'pkce-challenge';

const ops = {
  async challenge([method, verifier]) {
    return generateChallenge(verifier, method);
  },

  async verify([method, verifier, challenge]) {
    try {
      return await verifyChallenge(verifier, challenge, method);
    } catch (err) {
      if (/Unsupported PKCE challenge method/.test(String(err.message))) return false;
      throw err;
    }
  },

  async generatedProperties([method]) {
    const pair = await pkceChallenge(undefined, method);
    return {
      verifierLength: pair.code_verifier.length,
      method: pair.code_challenge_method,
      roundTrips: await verifyChallenge(pair.code_verifier, pair.code_challenge, method),
      challengeEqualsVerifier: pair.code_challenge === pair.code_verifier,
    };
  },
};

const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
// Requests are handled strictly in order: each line is chained onto the previous
// promise so the replies stay aligned with the harness's expectations.
let queue = Promise.resolve();
rl.on('line', (line) => {
  const text = line.trim();
  if (!text) return;
  queue = queue.then(async () => {
    let req;
    try {
      req = JSON.parse(text);
    } catch (err) {
      process.stdout.write(JSON.stringify({ id: null, ok: false, error: `bad request: ${err.message}` }) + '\n');
      return;
    }
    let out;
    try {
      const op = ops[req.fn];
      if (!op) throw new Error(`unknown fn ${req.fn}`);
      out = { id: req.id, ok: true, value: await op(req.args ?? []) };
    } catch (err) {
      out = { id: req.id, ok: false, error: String((err && err.message) || err) };
    }
    process.stdout.write(JSON.stringify(out) + '\n');
  });
});
