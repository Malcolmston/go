// Command run is the Go half of parity/passport/nested/pwhash: the same JSON
// Lines protocol as node/run.js, answered by
// github.com/malcolmston/passport/pwhash.
//
// Conventions shared with the upstream runner: passwords and salts are strings
// hashed as their UTF-8 bytes, derived keys are lowercase hex, every parameter
// (salt, iteration count, key length, algorithm) is explicit, and
// `verifyKnownKey` answers a verdict rather than an error.
//
// `verifyKnownKey` deliberately goes through the port's real credential path: it
// assembles the package's own encoded form —
// "pbkdf2_<alg>$<iterations>$<b64 salt>$<b64 derived key>" — and calls
// pwhash.Verify, so Decode's validation and Verify's constant-time comparison
// are both under test rather than a re-implementation of them.
package main

import (
	"bufio"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"os"
	"strings"

	"github.com/malcolmston/passport/pwhash"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID string `json:"id"`
	OK bool   `json:"ok"`
	// No omitempty: a legitimate answer may be false or "".
	Value any    `json:"value"`
	Error string `json:"error,omitempty"`
}

// hashFor maps the digest names the cases use onto the standard library
// constructors. An unknown name is an error, which is the answer for a hash
// neither side can compute.
func hashFor(name string) (func() hash.Hash, error) {
	switch name {
	case "sha1":
		return sha1.New, nil
	case "sha256":
		return sha256.New, nil
	case "sha512":
		return sha512.New, nil
	default:
		return nil, fmt.Errorf("pwhash parity: unsupported digest %q", name)
	}
}

func str(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("expected a string argument: %w", err)
	}
	return s, nil
}

func num(raw json.RawMessage) (int, error) {
	var n int
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, fmt.Errorf("expected a whole-number argument: %w", err)
	}
	return n, nil
}

func dispatch(req request) (any, error) {
	need := func(n int) error {
		if len(req.Args) < n {
			return fmt.Errorf("expected %d arguments, got %d", n, len(req.Args))
		}
		return nil
	}

	switch req.Fn {
	case "key":
		if err := need(5); err != nil {
			return nil, err
		}
		password, err := str(req.Args[0])
		if err != nil {
			return nil, err
		}
		salt, err := str(req.Args[1])
		if err != nil {
			return nil, err
		}
		iterations, err := num(req.Args[2])
		if err != nil {
			return nil, err
		}
		keyLen, err := num(req.Args[3])
		if err != nil {
			return nil, err
		}
		algName, err := str(req.Args[4])
		if err != nil {
			return nil, err
		}
		h, err := hashFor(algName)
		if err != nil {
			return nil, err
		}
		return hex.EncodeToString(pwhash.Key([]byte(password), []byte(salt), iterations, keyLen, h)), nil

	case "verifyKnownKey":
		if err := need(5); err != nil {
			return nil, err
		}
		password, err := str(req.Args[0])
		if err != nil {
			return nil, err
		}
		salt, err := str(req.Args[1])
		if err != nil {
			return nil, err
		}
		iterations, err := num(req.Args[2])
		if err != nil {
			return nil, err
		}
		algName, err := str(req.Args[3])
		if err != nil {
			return nil, err
		}
		expectedHex, err := str(req.Args[4])
		if err != nil {
			return nil, err
		}
		dk, err := hex.DecodeString(expectedHex)
		if err != nil {
			// A stored value that is not hex is a corrupt credential, which is
			// a rejection, not a crash.
			return false, nil
		}
		b64 := base64.RawStdEncoding.EncodeToString
		encoded := fmt.Sprintf("pbkdf2_%s$%d$%s$%s", algName, iterations, b64([]byte(salt)), b64(dk))
		ok, err := pwhash.Verify(password, encoded)
		if err != nil {
			// Malformed or unusable stored hash: a rejection.
			return false, nil
		}
		return ok, nil

	default:
		return nil, fmt.Errorf("unknown fn %q", req.Fn)
	}
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			emit(response{OK: false, Error: "bad request: " + err.Error()})
			continue
		}
		v, err := dispatch(req)
		if err != nil {
			emit(response{ID: req.ID, OK: false, Error: err.Error()})
			continue
		}
		emit(response{ID: req.ID, OK: true, Value: v})
	}
}

func emit(r response) {
	b, err := json.Marshal(r)
	if err != nil {
		b, _ = json.Marshal(response{ID: r.ID, OK: false, Error: "marshal: " + err.Error()})
	}
	_, _ = os.Stdout.Write(append(b, '\n'))
	_ = os.Stdout.Sync()
}
