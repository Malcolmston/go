// Old/alternate spellings of a library id, resolved to the canonical one.
//
// The repo directory name differs from the site id for exactly one library
// (repo socket.io, id socketio), and the dotted form is what the GitHub link,
// the docs bundle and the /parity/<slug> route all spell — so /lib/socket.io is
// a URL people actually write. The route permanently redirects it rather than
// serving the same library under two addresses (or, as it used to, a 200 dead
// end).
//
// It lives in its own module because a Next page file may only export the route
// exports Next knows about; anything else fails the generated route type check.
import { LIBS } from '../../../src/data';
import { repoKey } from '../../../src/parityLookup';

/** The canonical id for an alternate spelling, or null if `id` needs no redirect. */
export function aliasFor(id: string): string | null {
  if (LIBS.some((l) => l.id === id)) return null;
  const lib = LIBS.find((l) => repoKey(l) === id);
  return lib ? lib.id : null;
}
