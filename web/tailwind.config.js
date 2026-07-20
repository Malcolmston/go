/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    './app/**/*.{ts,tsx}',
    './src/**/*.{ts,tsx}',
    '../ui/src/**/*.{ts,tsx}',
  ],
  // The bespoke "Liquid Glass" design system ships its own reset, so disable
  // Tailwind's preflight to avoid disturbing the existing look.
  corePlugins: { preflight: false },
  theme: { extend: {} },
  plugins: [],
};
