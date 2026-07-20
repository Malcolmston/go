'use client';

import { useEffect, useState } from 'react';

const OWNER = 'malcolmston';
const GH: RequestInit = { headers: { Accept: 'application/vnd.github+json' } };

export interface VersionBadgeProps {
  repo: string;
  href?: string;
  className?: string;
}

interface Ver {
  tag: string;
  pre: boolean;
}

// VersionBadge fetches the latest release (falling back to the newest tag) for
// a repo and renders a green "vX.Y.Z · stable" pill (amber for pre-releases).
// Hidden until a version resolves, so it never shows a broken/empty state.
export function VersionBadge({ repo, href, className = '' }: VersionBadgeProps) {
  const [ver, setVer] = useState<Ver | null>(null);

  useEffect(() => {
    let alive = true;
    const apply = (tag: string | undefined, pre: boolean) => { if (alive && tag) setVer({ tag, pre }); };
    fetch(`https://api.github.com/repos/${OWNER}/${repo}/releases/latest`, GH)
      .then((r) => (r.ok ? r.json() : Promise.reject(r.status)))
      .then((rel) => apply(rel.tag_name, rel.prerelease))
      .catch(() =>
        fetch(`https://api.github.com/repos/${OWNER}/${repo}/tags?per_page=1`, GH)
          .then((r) => (r.ok ? r.json() : Promise.reject(r.status)))
          .then((tags) => { if (tags && tags.length) apply(tags[0].name, false); })
          .catch(() => {})
      );
    return () => { alive = false; };
  }, [repo]);

  if (!ver) return null;
  const cls = `ver show ${ver.pre ? 'pre' : ''} ${className}`.trim();
  const body = (
    <>
      <span className="dot" />{ver.tag}
      <span className="stable">· {ver.pre ? 'pre' : 'stable'}</span>
    </>
  );
  return href
    ? <a className={cls} href={href} target="_blank" rel="noopener" style={{ textDecoration: 'none' }}
         title={`${repo} ${ver.tag} — ${ver.pre ? 'pre-release' : 'stable'}`}>{body}</a>
    : <span className={cls} title={`${repo} ${ver.tag} — ${ver.pre ? 'pre-release' : 'stable'}`}>{body}</span>;
}
