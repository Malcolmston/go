import { describe, expect, it } from 'vitest';
import { devices } from '@playwright/test';

import config, {
  selectedDeviceNames,
  subsetNames,
  webServerCommand,
} from '../../playwright.config';

const allDevices = Object.keys(devices);

describe('playwright config: device matrix', () => {
  it('still ships the 100+ device descriptors the full sweep promises', () => {
    expect(allDevices.length).toBeGreaterThanOrEqual(100);
  });

  it('defaults to the small subset, not the full matrix', () => {
    const selected = selectedDeviceNames({});
    expect(selected).toEqual(subsetNames());
    expect(selected).toHaveLength(8);
    expect(selected.length).toBeLessThan(allDevices.length);
  });

  it('runs the full matrix only when E2E_FULL=1 is opted into', () => {
    expect(selectedDeviceNames({ E2E_FULL: '1' })).toEqual(allDevices);
    expect(selectedDeviceNames({ E2E_FULL: '0' })).toEqual(subsetNames());
    // The old, inverted gate must not resurrect the full matrix.
    expect(selectedDeviceNames({ E2E_SUBSET: '0' })).toEqual(subsetNames());
  });

  it('configures the subset as the default project list', () => {
    expect(config.projects?.map((p) => p.name)).toEqual(subsetNames());
  });
});

describe('playwright config: webServer', () => {
  it('serves an existing build instead of building inside webServer', () => {
    expect(webServerCommand({})).toBe('pnpm exec next start -p 4173');
    expect(webServerCommand({})).not.toContain('pnpm run build');
  });

  it('builds first only when E2E_BUILD=1', () => {
    expect(webServerCommand({ E2E_BUILD: '1' })).toBe(
      'pnpm run build && pnpm exec next start -p 4173',
    );
  });

  it('reuses a running server, pipes its output and allows a long startup', () => {
    const ws = config.webServer as {
      reuseExistingServer?: boolean;
      timeout?: number;
      stdout?: string;
      stderr?: string;
    };
    expect(ws.reuseExistingServer).toBe(true);
    expect(ws.timeout).toBeGreaterThanOrEqual(300_000);
    expect(ws.stdout).toBe('pipe');
    expect(ws.stderr).toBe('pipe');
  });

  it('retries once and caps workers regardless of CI', () => {
    expect(config.retries).toBe(1);
    expect(config.workers).toBe(4);
  });
});
