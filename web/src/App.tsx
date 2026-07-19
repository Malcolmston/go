import { useEffect } from 'react';
import type { ReactNode } from 'react';
import { Layout, useHashTab } from 'go-ui';
import type { Tab } from 'go-ui';
import { LIBS } from './data';
import { Home } from './components/Home';
import { LibView } from './components/LibView';
import { Parity } from './components/Parity';
import { Pipeline } from './components/Pipeline';
import { Releases } from './components/Releases';
import { HowTo } from './components/HowTo';
import { Faq } from './components/Faq';
import { AI } from './components/AI';
import { About } from './components/About';

const TABS: Tab[] = [
  { id: 'home', label: 'Home' },
  ...LIBS.map((l) => ({ id: l.id, label: l.name, dot: l.accent })),
  { id: 'parity', label: 'Parity' },
  { id: 'pipeline', label: 'Pipeline' },
  { id: 'releases', label: 'Releases' },
  { id: 'howto', label: 'How-to' },
  { id: 'faq', label: 'FAQ' },
  { id: 'ai', label: 'AI' },
  { id: 'about', label: 'About' },
];
const TAB_IDS = TABS.map((t) => t.id);

// App is the top-level composition: hash-routed tabs wrapped in the shared
// Layout, switching which view renders.
export function App() {
  const [active, go] = useHashTab(TAB_IDS, 'home');
  const lib = LIBS.find((l) => l.id === active);

  // Reveal-on-mount: make any .reveal blocks in the active view visible.
  useEffect(() => {
    const t = setTimeout(() => document.querySelectorAll('.reveal').forEach((el) => el.classList.add('in')), 30);
    return () => clearTimeout(t);
  }, [active]);

  const brand = { title: 'malcolmston', sub: '/go', initials: 'go', href: '#home' };
  const footer: ReactNode = (
    <div>
      <span className="grad-text" style={{ fontWeight: 700 }}>malcolmston/go</span> — the Node.js ecosystem, reimagined in Go.
      <div className="small dim" style={{ marginTop: '.4rem' }}>MIT licensed · independent re-implementations</div>
    </div>
  );

  return (
    <Layout brand={brand} tabs={TABS} active={active} onNav={go} github="https://github.com/malcolmston" footer={footer}>
      {active === 'home' && <Home go={go} />}
      {lib && <LibView lib={lib} key={lib.id} />}
      {active === 'parity' && <Parity />}
      {active === 'pipeline' && <Pipeline />}
      {active === 'releases' && <Releases />}
      {active === 'howto' && <HowTo />}
      {active === 'faq' && <Faq />}
      {active === 'ai' && <AI />}
      {active === 'about' && <About />}
    </Layout>
  );
}
