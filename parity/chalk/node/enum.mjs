import * as mod from 'chalk';
const c = mod.default;
console.log('MODULE EXPORTS:', JSON.stringify(Object.keys(mod).sort()));
const own = new Set([...Object.keys(c), ...Object.getOwnPropertyNames(c)]);
let p = Object.getPrototypeOf(c);
const protoNames = new Set();
while (p && p !== Object.prototype && p !== Function.prototype) {
  for (const n of Object.getOwnPropertyNames(p)) protoNames.add(n);
  p = Object.getPrototypeOf(p);
}
console.log('INSTANCE OWN:', JSON.stringify([...own].sort()));
console.log('PROTO:', JSON.stringify([...protoNames].sort()));
console.log('CHALK KEYS(for-in):', JSON.stringify(Object.keys(c).sort()));
