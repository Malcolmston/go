// go-ui — the shared Liquid-Glass React component library for the
// malcolmston/go documentation sites. Vendored into each library repo as a git
// submodule and imported directly from source (Vite bundles it), so there is no
// build/publish step.

import './styles.css';

export { CodeBlock } from './components/CodeBlock';
export type { CodeBlockProps } from './components/CodeBlock';
export { CompareCard } from './components/CompareCard';
export type { CompareCardProps } from './components/CompareCard';
export { VersionBadge } from './components/VersionBadge';
export type { VersionBadgeProps } from './components/VersionBadge';
export { ReleaseList } from './components/ReleaseList';
export type { ReleaseListProps, RelLib } from './components/ReleaseList';
export { LibReleases } from './components/LibReleases';
export type { LibReleasesProps } from './components/LibReleases';
export { Layout } from './components/Layout';
export type { LayoutProps, Tab, Brand } from './components/Layout';
export { ThemeToggle } from './components/ThemeToggle';
export { useHashTab } from './hooks/useHashTab';
export { applyStoredTheme } from './theme';
export { esc, hi, hx, ghrepo } from './highlight';
