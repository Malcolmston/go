import { forwardRef, useImperativeHandle, useRef, useEffect } from 'react';

// WormholeHandle is the imperative API: warp() plays a full-screen faster-than-
// light star tunnel, calls onArrive at the throat (mid-animation) to swap the
// page underneath, then decelerates out the other side.
export interface WormholeHandle {
  warp: (onArrive: () => void) => void;
}

const prefersReduced = () =>
  typeof matchMedia !== 'undefined' && matchMedia('(prefers-reduced-motion: reduce)').matches;

interface Star {
  x: number;
  y: number;
  z: number;
  pz: number; // previous z, for the streak
  white: boolean;
}

// Wormhole renders a fixed, pointer-transparent canvas that is transparent until
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
        // Already warping, or no canvas: just route immediately.
        if (!canvas || busy.current) {
          onArrive();
          return;
        }
        // Accessibility: honour reduced-motion with a brief glass flash only.
        if (prefersReduced()) {
          canvas.classList.add('on');
          onArrive();
          window.setTimeout(() => canvas.classList.remove('on'), 160);
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
        canvas.classList.add('on');

        const accent =
          getComputedStyle(document.documentElement).getPropertyValue('--accent').trim() || '#2f9bff';
        const purple =
          getComputedStyle(document.documentElement).getPropertyValue('--a-purple').trim() || '#b57bff';
        const count = Math.max(160, Math.min(620, Math.floor((W * H) / (2400 * dpr))));
        const rand = (a: number, b: number) => a + Math.random() * (b - a);
        const spawn = (): Star => {
          const z = rand(1, W);
          return { x: rand(-W, W), y: rand(-H, H), z, pz: z, white: Math.random() < 0.5 };
        };
        const stars: Star[] = Array.from({ length: count }, spawn);

        const DURATION = 820;
        const t0 = performance.now();
        let arrived = false;

        const frame = (t: number) => {
          const p = Math.min(1, (t - t0) / DURATION);
          const ease = Math.sin(Math.PI * p); // 0 → 1 (at p=0.5) → 0
          const speed = 8 + 64 * ease; // accelerate into the throat, decelerate out

          // Motion-blur trail: paint a translucent dark wash each frame.
          ctx.fillStyle = 'rgba(4,6,12,0.32)';
          ctx.fillRect(0, 0, W, H);

          for (const s of stars) {
            s.pz = s.z;
            s.z -= speed;
            if (s.z < 1) {
              const n = spawn();
              s.x = n.x;
              s.y = n.y;
              s.z = W;
              s.pz = s.z;
            }
            const sx = cx + (s.x / s.z) * cx;
            const sy = cy + (s.y / s.z) * cy;
            const px = cx + (s.x / s.pz) * cx;
            const py = cy + (s.y / s.pz) * cy;
            ctx.strokeStyle = s.white ? 'rgba(255,255,255,0.92)' : Math.random() < 0.5 ? accent : purple;
            ctx.lineWidth = Math.max(0.6, (1 - s.z / W) * 3.2) * dpr;
            ctx.beginPath();
            ctx.moveTo(px, py);
            ctx.lineTo(sx, sy);
            ctx.stroke();
          }

          // A bright flash at the throat of the wormhole (peaks at p=0.5).
          if (p > 0.4 && p < 0.6) {
            const f = 1 - Math.abs(p - 0.5) / 0.1;
            ctx.fillStyle = `rgba(255,255,255,${f * 0.85})`;
            ctx.fillRect(0, 0, W, H);
          }

          // Swap the page at the throat, so the reader "arrives" through the light.
          if (!arrived && p >= 0.5) {
            arrived = true;
            onArrive();
          }

          if (p < 1) {
            raf.current = requestAnimationFrame(frame);
          } else {
            ctx.clearRect(0, 0, W, H);
            canvas.classList.remove('on');
            busy.current = false;
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
