import { ReleaseList, ghrepo } from 'go-ui';
import type { RelLib } from 'go-ui';
import { LIBS } from '../data';
import { SecH } from './SecH';

const GO_ACCENT = '#2f9bff';
const RELEASE_LIBS: RelLib[] = LIBS.map((l) => ({ name: l.name, icon: l.icon, accent: l.accent, repo: ghrepo(l), url: l.repo }))
  .concat([{ name: 'malcolmston/go', icon: '<i class="fa-brands fa-golang"></i>', accent: GO_ACCENT, repo: 'go', url: 'https://github.com/malcolmston' }]);

// Releases renders the live release-history + changelog tab.
export function Releases() {
  return (
    <section className="view active" id="view-releases">
      <SecH h="h2">Releases &amp; changelogs</SecH>
      <p className="muted">Every library ships automated semver releases — the moment a <code>VERSION</code> bump lands on <code>main</code>, a tag and GitHub Release are cut and the moving <code>stable</code> tag advances. The list below is read <b>live</b> from the GitHub Releases API, newest first, so it is never out of date. Full history for each library lives in its <code>CHANGELOG.md</code>.</p>
      <div style={{ marginTop: '1.4rem' }}><ReleaseList libs={RELEASE_LIBS} /></div>
    </section>
  );
}
