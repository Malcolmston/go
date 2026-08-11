import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Security, type SecurityData, type SecurityFinding } from '../../../src/components/Security';

// A finding shaped exactly like scripts/build-security-data.ts emits one.
function finding(over: Partial<SecurityFinding>): SecurityFinding {
  return {
    id: 'a-finding',
    library: 'express',
    nestedPackage: null,
    port: 'express',
    repo: 'express',
    module: 'github.com/malcolmston/express',
    manifestPath: 'parity/express/security.json',
    severity: 'high',
    summary: 'Something unsafe',
    affectedVersions: '<= 0.4.0',
    cases: ['case-one'],
    ghsa: null,
    description: '## Affected\n\nfull note',
    scopeNote: null,
    upstream: 'express@4.21.2',
    upstreamEcosystem: 'npm',
    releasedVersion: '0.5.0',
    status: 'fixed',
    fixedIn: '0.5.0',
    statusReason: 'released 0.5.0 is outside "<= 0.4.0"',
    ...over,
  };
}

const FIXED = finding({ id: 'fixed-one' });
const UNFIXED = finding({
  id: 'unfixed-one',
  library: 'streamlit',
  port: 'streamlit',
  severity: 'medium',
  affectedVersions: '<= 0.3.0',
  releasedVersion: '0.3.0',
  status: 'unfixed',
  fixedIn: null,
  statusReason: 'released 0.3.0 is inside "<= 0.3.0"',
});
const NESTED = finding({
  id: 'nested-one',
  nestedPackage: 'sanitizehtml',
  port: 'express/sanitizehtml',
  severity: 'low',
  upstream: 'sanitize-html@2.17.6',
  scopeNote: 'An earlier draft claimed something that direct execution disproves.',
});

const DATA: SecurityData = {
  generatedAt: '2026-01-01T00:00:00.000Z',
  statusDerivation: 'Derived from the affected range and the VERSION file.',
  counts: { total: 3, bySeverity: { high: 1, medium: 1, low: 1 }, fixed: 2, unfixed: 1, unknown: 0, libraries: 2 },
  findings: [FIXED, UNFIXED, NESTED],
};

describe('Security', () => {
  it('renders one card per finding with its id, range and cases', () => {
    const { container } = render(<Security data={DATA} />);
    expect(container.querySelector('#view-security')).not.toBeNull();
    expect(container.querySelectorAll('.sec-card').length).toBe(3);
    expect(screen.getByText('unfixed-one')).toBeInTheDocument();
    expect(screen.getAllByText('<= 0.4.0').length).toBe(2);
    expect(screen.getByText('<= 0.3.0')).toBeInTheDocument();
    expect(screen.getAllByText('case-one').length).toBe(3);
  });

  it('counts fixed and unfixed from the data rather than from prose', () => {
    const { container } = render(<Security data={DATA} />);
    const labels = Array.from(container.querySelectorAll('.sec-tile')).map((t) => t.textContent ?? '');
    expect(labels.some((t) => /^2fixed in the latest release/.test(t))).toBe(true);
    expect(labels.some((t) => /^1unfixed in the latest release/.test(t))).toBe(true);
  });

  it('says plainly when the latest release is still affected', () => {
    render(<Security data={DATA} />);
    expect(screen.getAllByText('unfixed in the latest release').length).toBeGreaterThan(0);
    expect(screen.getAllByText('fixed in 0.5.0', { selector: '.sec-pill' }).length).toBe(2);
  });

  it('shows the per-finding derivation instead of asserting a status', () => {
    render(<Security data={DATA} />);
    expect(screen.getByText(/released 0\.3\.0 is inside/)).toBeInTheDocument();
    expect(screen.getByText(/Derived from the affected range and the VERSION file/)).toBeInTheDocument();
  });

  it('surfaces a scope note and explains the nested port', () => {
    render(<Security data={DATA} />);
    expect(screen.getByText(/An earlier draft claimed something/)).toBeInTheDocument();
    expect(screen.getByText('express/sanitizehtml')).toBeInTheDocument();
    expect(screen.getByText(/sanitize-html@2\.17\.6/)).toBeInTheDocument();
  });

  it('explains the draft-advisory disclosure policy once', () => {
    render(<Security data={DATA} />);
    expect(screen.getAllByText(/Publishing a draft is always a manual decision/).length).toBe(1);
  });

  it('filters by severity and by status', async () => {
    const user = userEvent.setup();
    const { container } = render(<Security data={DATA} />);
    await user.selectOptions(screen.getByLabelText('Severity'), 'medium');
    expect(container.querySelectorAll('.sec-card').length).toBe(1);
    expect(screen.getByText('unfixed-one')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Clear filters' }));
    expect(container.querySelectorAll('.sec-card').length).toBe(3);

    await user.selectOptions(screen.getByLabelText('Status'), 'fixed');
    expect(container.querySelectorAll('.sec-card').length).toBe(2);
    expect(screen.queryByText('unfixed-one')).toBeNull();
  });

  it('filters by library', async () => {
    const user = userEvent.setup();
    const { container } = render(<Security data={DATA} />);
    await user.selectOptions(screen.getByLabelText('Library'), 'streamlit');
    expect(container.querySelectorAll('.sec-card').length).toBe(1);
  });

  it('renders an empty record without implying a clean bill of health', () => {
    render(<Security data={{ ...DATA, findings: [] }} />);
    expect(screen.getByText(/No recorded finding matches this filter/)).toBeInTheDocument();
  });
});
