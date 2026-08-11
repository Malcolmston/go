// Shared fixture for the Parity page tests.
//
// The real dataset is generated into api/_data/parity.json from the harnesses
// under parity/, and the components take it as props (the page reads the file on
// the server), so the tests build the same shape by hand — small, but with every
// status represented so the filters have something to filter.
import { normalizeParityData, summarize } from '../../../src/components/ParityData';
import type { ParityData, ParityHarness, ParitySummary } from '../../../src/components/ParityData';

const RAW = {
  generatedAt: '2026-08-11T09:23:04Z',
  libraries: {
    express: {
      library: 'express',
      harnessPath: 'parity/express',
      upstream: { raw: 'express@4.19.2', name: 'express', version: '4.19.2', ecosystem: 'node' },
      goModule: 'github.com/malcolmston/express',
      goVersion: 'v0.5.0',
      generatedBy: 'go test ./parity/express',
      total: 4,
      match: 2,
      mismatch: 1,
      deviations: 1,
      parityPercent: 50,
      comparedTotal: 4,
      groups: [{ name: 'router', total: 4, match: 2, mismatch: 1 }],
      cases: [
        { id: 'get-basic', group: 'router', fn: 'get', upstreamFn: 'app.get', goFn: 'App.Get', note: '', deviation: '', status: 'match' },
        { id: 'get-regex', group: 'router', fn: 'get', upstreamFn: 'app.get', goFn: 'App.Get', note: '', deviation: '', status: 'match' },
        { id: 'etag-weak', group: 'router', fn: 'etag', upstreamFn: 'app.etag', goFn: 'App.ETag', note: 'weak etag differs', deviation: '', status: 'mismatch' },
        { id: 'json-order', group: 'router', fn: 'json', upstreamFn: 'res.json', goFn: 'Res.JSON', note: '', deviation: 'Go sorts map keys', status: 'deviation' },
      ],
      coverage: [
        { upstreamSymbol: 'app.get', goSymbol: 'App.Get', status: 'match', cases: ['get-basic', 'get-regex'], note: '' },
        { upstreamSymbol: 'app.etag', goSymbol: 'App.ETag', status: 'differs', cases: ['etag-weak'], note: 'weak etag differs' },
        { upstreamSymbol: 'app.mkactivity', goSymbol: '', status: 'missing', cases: [], note: 'not ported' },
        { upstreamSymbol: '', goSymbol: 'App.MustParse', status: 'extra', cases: [], note: 'Go-only helper' },
        { upstreamSymbol: 'app.render', goSymbol: 'App.Render', status: 'untested', cases: [], note: '' },
      ],
      coverageCommand: "node -e \"console.log(Object.keys(require('express')))\"",
      security: [
        {
          id: 'etag-weak-bypass',
          summary: 'Weak ETag comparison admits a stale body',
          severity: 'high',
          affectedVersions: '<= 0.5.0',
          cases: ['etag-weak'],
        },
      ],
      nested: {
        qs: {
          library: 'qs',
          harnessPath: 'parity/express/nested/qs',
          upstream: { raw: 'qs@6.11.2', name: 'qs', version: '6.11.2', ecosystem: 'node' },
          goModule: 'github.com/malcolmston/express/qs',
          goVersion: 'v0.5.0',
          generatedBy: 'go test ./parity/express/nested/qs',
          total: 2,
          match: 2,
          mismatch: 0,
          deviations: 0,
          parityPercent: 100,
          comparedTotal: 2,
          groups: [{ name: 'parse', total: 2, match: 2, mismatch: 0 }],
          cases: [
            { id: 'parse-nested', group: 'parse', fn: 'parse', upstreamFn: 'qs.parse', goFn: 'qs.Parse', note: '', deviation: '', status: 'match' },
            { id: 'stringify-arr', group: 'parse', fn: 'stringify', upstreamFn: 'qs.stringify', goFn: 'qs.Stringify', note: '', deviation: '', status: 'match' },
          ],
          coverage: [
            { upstreamSymbol: 'qs.parse', goSymbol: 'qs.Parse', status: 'match', cases: ['parse-nested'], note: '' },
          ],
          coverageCommand: "node -e \"console.log(Object.keys(require('qs')))\"",
          security: [],
        },
      },
    },
    numpy: {
      library: 'numpy',
      harnessPath: 'parity/numpy',
      upstream: { raw: 'numpy==2.0.1', name: 'numpy', version: '2.0.1', ecosystem: 'python' },
      goModule: 'github.com/malcolmston/numpy',
      goVersion: 'v0.2.0',
      generatedBy: 'go test ./parity/numpy',
      total: 3,
      match: 3,
      mismatch: 0,
      deviations: 0,
      parityPercent: 100,
      comparedTotal: 3,
      groups: [{ name: 'ufunc', total: 3, match: 3, mismatch: 0 }],
      cases: [
        { id: 'add-int', group: 'ufunc', fn: 'add', upstreamFn: 'numpy.add', goFn: 'numpy.Add', note: '', deviation: '', status: 'match' },
      ],
      coverage: [
        { upstreamSymbol: 'numpy.add', goSymbol: 'numpy.Add', status: 'match', cases: ['add-int'], note: '' },
      ],
      coverageCommand: 'python -c "import numpy; print(dir(numpy))"',
      security: [],
      nested: {},
    },
  },
};

export function parityFixture(): ParityData {
  return normalizeParityData(RAW);
}

export function harnessFixture(id: 'express' | 'numpy' = 'express'): ParityHarness {
  return parityFixture().libraries[id];
}

const ACCENTS: Record<string, string> = { express: '#38bdf8', numpy: '#f97316' };
const NAMES: Record<string, string> = { express: 'Express', numpy: 'NumPy' };

export function summaryFixture(): ParitySummary[] {
  const data = parityFixture();
  return Object.entries(data.libraries).map(([slug, h]) =>
    summarize(slug, h, NAMES[slug] ?? slug, ACCENTS[slug] ?? '#888888'),
  );
}
