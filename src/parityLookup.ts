import { PARITY } from './parity';
import type { Parity } from './parity';
import type { Lib } from './data';

// parityFor resolves a library's parity record. parity.ts is regenerated from
// each repo's parity.json (keyed by the repo's `repo` field, i.e. its directory
// name), while Lib.id is the landing's own slug — these match for every library
// except socket.io (id "socketio", repo "socket.io"). Try the id first, then the
// repo basename, so the lookup is robust to that (and any future) mismatch.
export function parityFor(lib: Lib): Parity | undefined {
  return PARITY[lib.id] ?? PARITY[repoKey(lib)];
}

// repoKey is the last path segment of the library's GitHub repo URL.
export function repoKey(lib: Lib): string {
  return lib.repo.replace(/\/+$/, '').split('/').pop() ?? lib.id;
}
