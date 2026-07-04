import { LibReleases } from './LibReleases';

export interface RelLib {
  name: string;
  icon: string;
  accent: string;
  repo: string;
  url: string;
}

export interface ReleaseListProps {
  libs: RelLib[];
}

// ReleaseList renders a live release-history + notes block per library, read
// from the GitHub Releases API. `libs` is [{ name, icon, accent, repo, url }].
export function ReleaseList({ libs }: ReleaseListProps) {
  return (
    <div id="rellist">
      {libs.map((lib) => <LibReleases key={lib.repo} lib={lib} />)}
    </div>
  );
}
