import { describe, it, expect } from 'vitest';
import { isTopLevel, labelForTab, pathForTab, tabForPath } from '../../../app/nav';
import { LIBS } from '../../../src/data';

// app/nav.ts is the single source of truth for the tab-id <-> route mapping the
// shell uses to highlight the sidebar and to route the Layout's onNav.
describe('nav', () => {
  describe('pathForTab', () => {
    it('maps home to the root path', () => {
      expect(pathForTab('home')).toBe('/');
    });

    it('maps each fixed section to its own top-level path', () => {
      for (const id of ['parity', 'explore', 'releases', 'howto', 'faq', 'ai', 'ask', 'about']) {
        expect(pathForTab(id)).toBe(`/${id}`);
      }
    });

    it('routes every library under /lib/', () => {
      for (const lib of LIBS) {
        expect(pathForTab(lib.id)).toBe(`/lib/${lib.id}`);
      }
    });

    it('percent-encodes an id that is not URL-safe', () => {
      expect(pathForTab('a/b')).toBe('/lib/a%2Fb');
    });
  });

  describe('tabForPath', () => {
    it('treats a missing pathname as home', () => {
      expect(tabForPath(null)).toBe('home');
      expect(tabForPath(undefined)).toBe('home');
      expect(tabForPath('')).toBe('home');
      expect(tabForPath('/')).toBe('home');
    });

    it('ignores a trailing slash from the static export', () => {
      expect(tabForPath('/faq/')).toBe('faq');
      expect(tabForPath('/lib/express/')).toBe('express');
    });

    it('decodes an encoded library segment', () => {
      expect(tabForPath('/lib/socket%2Eio')).toBe('socket.io');
    });

    it('falls back to home for an unknown top-level segment', () => {
      expect(tabForPath('/nope')).toBe('home');
    });

    // /pipeline was folded into /parity (the pipeline is shown next to the
    // scores it produces). The old URL must still highlight a real tab, and a
    // per-library parity record belongs to the Parity tab too.
    it('resolves the retired pipeline route and per-library records to Parity', () => {
      expect(tabForPath('/pipeline')).toBe('parity');
      expect(tabForPath('/pipeline/')).toBe('parity');
      expect(pathForTab('pipeline')).toBe('/parity');
      expect(tabForPath('/parity/express')).toBe('parity');
    });

    it('round-trips every library id', () => {
      for (const lib of LIBS) {
        expect(tabForPath(pathForTab(lib.id))).toBe(lib.id);
      }
    });
  });

  describe('labelForTab', () => {
    it('labels home and the fixed sections', () => {
      expect(labelForTab('home')).toBe('Home');
      expect(labelForTab('faq')).toBe('FAQ');
      expect(labelForTab('howto')).toBe('How-to');
    });

    it('labels a library with its display name', () => {
      expect(labelForTab('socketio')).toBe('Socket.IO');
      expect(labelForTab('express')).toBe('Express');
    });

    it('falls back to the raw id for something unknown', () => {
      expect(labelForTab('not-a-thing')).toBe('not-a-thing');
    });
  });

  describe('isTopLevel', () => {
    it('separates fixed sections from libraries', () => {
      expect(isTopLevel('parity')).toBe(true);
      expect(isTopLevel('express')).toBe(false);
      expect(isTopLevel('home')).toBe(false);
    });
  });
});
