'use client';

import { forwardRef, useImperativeHandle, useRef, useEffect } from 'react';

// WormholeHandle is the imperative API: warp() plays a full-screen, OPAQUE
// faster-than-light star tunnel, calls onArrive at the throat (mid-animation)
// to swap the page underneath while the overlay hides it, then dissolves out to
// reveal the new page. Because the overlay is opaque (not blended over the live
// page) the effect reads as a clean transition rather than noise over text.
export interface WormholeHandle {
  warp: (onArrive: () => void) => void;
}

const prefersReduced = () =>
  typeof matchMedia !== 'undefined' && matchMedia('(prefers-reduced-motion: reduce)').matches;

interface Star {
  a: number; // angle
  r: number; // current radius (0 = centre)
  pr: number; // previous radius, for the streak
  hue: 0 | 1 | 2; // 0 white, 1 accent, 2 purple
  len: number; // streak length factor
}

// Wormhole renders a fixed, pointer-transparent canvas that is invisible until
// warp() is called. Mount it once (Layout does). It never re-renders React.
export const Wormhole = forwardRef<WormholeHandle>(function Wormhole(_props, ref) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const raf = useRef(0);
  const busy = useRef(false);

  useImperativeHandle(
    ref,
    () => ({
      warp(onArrive: () => void) {
        const canvas = canvasRef.current;
        if (!canvas || busy.current) {
          onArrive();
          return;
        }
        // Reduced motion: a brief, calm cross-dissolve — no motion.
        if (prefersReduced()) {
          const ctx0 = canvas.getContext('2d');
          if (ctx0) {
            canvas.width = canvas.clientWidth || window.innerWidth;
            canvas.height = canvas.clientHeight || window.innerHeight;
            ctx0.fillStyle = '#06080f';
            ctx0.fillRect(0, 0, canvas.width, canvas.height);
          }
          canvas.classList.add('on');
          onArrive();
          window.setTimeout(() => {
            canvas.classList.remove('on');
            ctx0?.clearRect(0, 0, canvas.width, canvas.height);
          }, 180);
          return;
        }
        const ctx = canvas.getContext('2d');
        if (!ctx) {
          onArrive();
          return;
        }

        busy.current = true;
        const dpr = Math.min(window.devicePixelRatio || 1, 2);
        const W = (canvas.width = Math.max(1, Math.floor(window.innerWidth * dpr)));
        const H = (canvas.height = Math.max(1, Math.floor(window.innerHeight * dpr)));
        const cx = W / 2;
        const cy = H / 2;
        const maxR = Math.hypot(cx, cy);
        canvas.classList.add('on');

        const css = getComputedStyle(document.documentElement);
        const accent = css.getPropertyValue('--accent').trim() || '#2f9bff';
        const purple = css.getPropertyValue('--a-purple').trim() || '#b57bff';
        const colors = ['rgba(255,255,255,', accent, purple];

        const count = Math.max(220, Math.min(900, Math.floor((W * H) / (1500 * dpr))));
        const rand = (a: number, b: number) => a + Math.random() * (b - a);
        const spawn = (): Star => ({
          a: rand(0, Math.PI * 2),
          r: rand(0, maxR),
          pr: 0,
          hue: (Math.random() < 0.6 ? 0 : Math.random() < 0.5 ? 1 : 2) as 0 | 1 | 2,
          len: rand(0.6, 1.4),
        });
        const stars: Star[] = Array.from({ length: count }, () => {
          const s = spawn();
          s.pr = s.r;
          return s;
        });

        // Pre-build the opaque space backdrop once (radial: navy core → near-black
        // rim) so every frame is a cheap fill and the page is fully hidden.
        const bg = ctx.createRadialGradient(cx, cy, 0, cx, cy, maxR);
        bg.addColorStop(0, '#0b1226');
        bg.addColorStop(0.6, '#070a16');
        bg.addColorStop(1, '#04060d');

        const DURATION = 760;
        const t0 = performance.now();
        let arrived = false;
        // smootherstep for graceful accel/decel
        const smooth = (x: number) => x * x * x * (x * (x * 6 - 15) + 10);

        const frame = (t: number) => {
          const p = Math.min(1, (t - t0) / DURATION);
          const pulse = Math.sin(Math.PI * p); // 0 → 1 (throat) → 0
          const speed = (0.5 + 22 * smooth(pulse)) * dpr; // px/frame outward

          // Opaque backdrop — hides the page underneath.
          ctx.globalCompositeOperation = 'source-over';
          ctx.fillStyle = bg;
          ctx.fillRect(0, 0, W, H);

          // Additive star streaks radiating out from the centre.
          ctx.globalCompositeOperation = 'lighter';
          for (const s of stars) {
            s.pr = s.r;
            s.r += speed * (s.r / maxR + 0.15) * 4;
            if (s.r > maxR) {
              s.a = rand(0, Math.PI * 2);
              s.r = rand(0, maxR * 0.12);
              s.pr = 0;
            }
            const cos = Math.cos(s.a);
            const sin = Math.sin(s.a);
            const sx = cx + cos * s.r;
            const sy = cy + sin * s.r;
            const tail = Math.min(s.r, (s.r - s.pr) * 3 * s.len + 2 * dpr);
            const px = cx + cos * (s.r - tail);
            const py = cy + sin * (s.r - tail);
            const depth = s.r / maxR; // 0 centre → 1 rim
            const alpha = 0.15 + 0.85 * depth;
            const col = colors[s.hue];
            ctx.strokeStyle = s.hue === 0 ? `${col}${alpha})` : col;
            ctx.globalAlpha = s.hue === 0 ? 1 : alpha;
            ctx.lineWidth = Math.max(0.6, depth * 2.4) * dpr;
            ctx.beginPath();
            ctx.moveTo(px, py);
            ctx.lineTo(sx, sy);
            ctx.stroke();
          }
          ctx.globalAlpha = 1;

          // Soft bloom at the throat — a warm glow, never a hard white flash.
          const glowR = maxR * (0.12 + 0.5 * pulse);
          const glow = ctx.createRadialGradient(cx, cy, 0, cx, cy, glowR);
          glow.addColorStop(0, `rgba(214,232,255,${0.55 * pulse})`);
          glow.addColorStop(0.5, `rgba(120,170,255,${0.28 * pulse})`);
          glow.addColorStop(1, 'rgba(120,170,255,0)');
          ctx.fillStyle = glow;
          ctx.fillRect(0, 0, W, H);

          // Swap the page at the throat, hidden behind the opaque overlay.
          if (!arrived && p >= 0.5) {
            arrived = true;
            onArrive();
            window.scrollTo(0, 0);
          }

          if (p < 1) {
            raf.current = requestAnimationFrame(frame);
          } else {
            // Dissolve out (CSS opacity), then clear so the new page shows through.
            canvas.classList.remove('on');
            window.setTimeout(() => {
              ctx.clearRect(0, 0, W, H);
              busy.current = false;
            }, 240);
          }
        };
        raf.current = requestAnimationFrame(frame);
      },
    }),
    [],
  );

  useEffect(() => () => cancelAnimationFrame(raf.current), []);

  return <canvas ref={canvasRef} className="wormhole" aria-hidden="true" />;
});
