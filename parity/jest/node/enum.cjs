// Mechanically enumerate the expect@29.7.0 matcher surface.
const {expect} = require('expect');
const proto = Object.keys(expect(0)).sort();
const statics = Object.keys(expect).sort();
console.log('## matchers on expect(0)  (' + proto.length + ')');
console.log(proto.join('\n'));
console.log('\n## statics on expect  (' + statics.length + ')');
console.log(statics.join('\n'));
console.log('\n## expect.not  (' + Object.keys(expect.not).sort().length + ')');
console.log(Object.keys(expect.not).sort().join('\n'));
