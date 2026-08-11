// Code generated from d3-scale-chromatic v3.1.0. DO NOT EDIT.
//
// The specifier strings below are copied verbatim from the upstream bundle at
// https://unpkg.com/d3-scale-chromatic@3.1.0/dist/d3-scale-chromatic.js, which
// carries the ColorBrewer copyright: 2002 Cynthia Brewer, Mark Harrower and The
// Pennsylvania State University. Keeping them in d3's own packed form — six hex
// digits per color, one string per k — is deliberate: it makes this file a
// character-for-character diff against upstream, which is the only practical way
// to audit a table where a single wrong digit is invisible to every test.

package scalechromatic

// Categorical schemes. Each is a fixed list of visually distinct colors for
// unordered data, meant to be handed to [scale.Ordinal] rather than sampled.

// SchemeCategory10 is d3's default categorical scheme: the ten Tableau-derived hues used by
// every d3 example that does not say otherwise.
// It has 10 colors.
var SchemeCategory10 = colors("1f77b4ff7f0e2ca02cd627289467bd8c564be377c27f7f7fbcbd2217becf")

// SchemeAccent is ColorBrewer "Accent" — eight pastel-with-accents qualitative colors.
// It has 8 colors.
var SchemeAccent = colors("7fc97fbeaed4fdc086ffff99386cb0f0027fbf5b17666666")

// SchemeDark2 is ColorBrewer "Dark2" — eight dark qualitative colors, print-safe.
// It has 8 colors.
var SchemeDark2 = colors("1b9e77d95f027570b3e7298a66a61ee6ab02a6761d666666")

// SchemePaired is ColorBrewer "Paired" — twelve colors in six light/dark pairs, for data
// with a natural two-level grouping.
// It has 12 colors.
var SchemePaired = colors("a6cee31f78b4b2df8a33a02cfb9a99e31a1cfdbf6fff7f00cab2d66a3d9affff99b15928")

// SchemePastel1 is ColorBrewer "Pastel1" — nine pale qualitative colors.
// It has 9 colors.
var SchemePastel1 = colors("fbb4aeb3cde3ccebc5decbe4fed9a6ffffcce5d8bdfddaecf2f2f2")

// SchemePastel2 is ColorBrewer "Pastel2" — eight pale qualitative colors.
// It has 8 colors.
var SchemePastel2 = colors("b3e2cdfdcdaccbd5e8f4cae4e6f5c9fff2aef1e2cccccccc")

// SchemeSet1 is ColorBrewer "Set1" — nine saturated qualitative colors.
// It has 9 colors.
var SchemeSet1 = colors("e41a1c377eb84daf4a984ea3ff7f00ffff33a65628f781bf999999")

// SchemeSet2 is ColorBrewer "Set2" — eight muted qualitative colors.
// It has 8 colors.
var SchemeSet2 = colors("66c2a5fc8d628da0cbe78ac3a6d854ffd92fe5c494b3b3b3")

// SchemeSet3 is ColorBrewer "Set3" — twelve pale qualitative colors.
// It has 12 colors.
var SchemeSet3 = colors("8dd3c7ffffb3bebadafb807280b1d3fdb462b3de69fccde5d9d9d9bc80bdccebc5ffed6f")

// SchemeTableau10 is Tableau's ten-color categorical palette.
// It has 10 colors.
var SchemeTableau10 = colors("4e79a7f28e2ce1575976b7b259a14fedc949af7aa1ff9da79c755fbab0ab")

// SchemeObservable10 is Observable's ten-color categorical palette.
// It has 10 colors.
var SchemeObservable10 = colors("4269d0efb118ff725c6cc5b03ca951ff8ab7a463f297bbf59c6b4e9498a0")

// Diverging families. Each is published for k = 3 to 11 and is symmetric about a
// pale midpoint, so the middle class reads as "neither" rather than as a value.

// SchemeBrBG is the ColorBrewer "BrBG" family, running brown to blue-green.
// Indexable by class count k for k = 3 to 11.
var SchemeBrBG = family(
	"d8b365f5f5f55ab4ac",                                                 // k = 3
	"a6611adfc27d80cdc1018571",                                           // k = 4
	"a6611adfc27df5f5f580cdc1018571",                                     // k = 5
	"8c510ad8b365f6e8c3c7eae55ab4ac01665e",                               // k = 6
	"8c510ad8b365f6e8c3f5f5f5c7eae55ab4ac01665e",                         // k = 7
	"8c510abf812ddfc27df6e8c3c7eae580cdc135978f01665e",                   // k = 8
	"8c510abf812ddfc27df6e8c3f5f5f5c7eae580cdc135978f01665e",             // k = 9
	"5430058c510abf812ddfc27df6e8c3c7eae580cdc135978f01665e003c30",       // k = 10
	"5430058c510abf812ddfc27df6e8c3f5f5f5c7eae580cdc135978f01665e003c30", // k = 11
)

// SchemePRGn is the ColorBrewer "PRGn" family, running purple to green.
// Indexable by class count k for k = 3 to 11.
var SchemePRGn = family(
	"af8dc3f7f7f77fbf7b",                                                 // k = 3
	"7b3294c2a5cfa6dba0008837",                                           // k = 4
	"7b3294c2a5cff7f7f7a6dba0008837",                                     // k = 5
	"762a83af8dc3e7d4e8d9f0d37fbf7b1b7837",                               // k = 6
	"762a83af8dc3e7d4e8f7f7f7d9f0d37fbf7b1b7837",                         // k = 7
	"762a839970abc2a5cfe7d4e8d9f0d3a6dba05aae611b7837",                   // k = 8
	"762a839970abc2a5cfe7d4e8f7f7f7d9f0d3a6dba05aae611b7837",             // k = 9
	"40004b762a839970abc2a5cfe7d4e8d9f0d3a6dba05aae611b783700441b",       // k = 10
	"40004b762a839970abc2a5cfe7d4e8f7f7f7d9f0d3a6dba05aae611b783700441b", // k = 11
)

// SchemePiYG is the ColorBrewer "PiYG" family, running pink to yellow-green.
// Indexable by class count k for k = 3 to 11.
var SchemePiYG = family(
	"e9a3c9f7f7f7a1d76a",                                                 // k = 3
	"d01c8bf1b6dab8e1864dac26",                                           // k = 4
	"d01c8bf1b6daf7f7f7b8e1864dac26",                                     // k = 5
	"c51b7de9a3c9fde0efe6f5d0a1d76a4d9221",                               // k = 6
	"c51b7de9a3c9fde0eff7f7f7e6f5d0a1d76a4d9221",                         // k = 7
	"c51b7dde77aef1b6dafde0efe6f5d0b8e1867fbc414d9221",                   // k = 8
	"c51b7dde77aef1b6dafde0eff7f7f7e6f5d0b8e1867fbc414d9221",             // k = 9
	"8e0152c51b7dde77aef1b6dafde0efe6f5d0b8e1867fbc414d9221276419",       // k = 10
	"8e0152c51b7dde77aef1b6dafde0eff7f7f7e6f5d0b8e1867fbc414d9221276419", // k = 11
)

// SchemePuOr is the ColorBrewer "PuOr" family, running orange to purple.
// Indexable by class count k for k = 3 to 11.
var SchemePuOr = family(
	"998ec3f7f7f7f1a340",                                                 // k = 3
	"5e3c99b2abd2fdb863e66101",                                           // k = 4
	"5e3c99b2abd2f7f7f7fdb863e66101",                                     // k = 5
	"542788998ec3d8daebfee0b6f1a340b35806",                               // k = 6
	"542788998ec3d8daebf7f7f7fee0b6f1a340b35806",                         // k = 7
	"5427888073acb2abd2d8daebfee0b6fdb863e08214b35806",                   // k = 8
	"5427888073acb2abd2d8daebf7f7f7fee0b6fdb863e08214b35806",             // k = 9
	"2d004b5427888073acb2abd2d8daebfee0b6fdb863e08214b358067f3b08",       // k = 10
	"2d004b5427888073acb2abd2d8daebf7f7f7fee0b6fdb863e08214b358067f3b08", // k = 11
)

// SchemeRdBu is the ColorBrewer "RdBu" family, running red to blue.
// Indexable by class count k for k = 3 to 11.
var SchemeRdBu = family(
	"ef8a62f7f7f767a9cf",                                                 // k = 3
	"ca0020f4a58292c5de0571b0",                                           // k = 4
	"ca0020f4a582f7f7f792c5de0571b0",                                     // k = 5
	"b2182bef8a62fddbc7d1e5f067a9cf2166ac",                               // k = 6
	"b2182bef8a62fddbc7f7f7f7d1e5f067a9cf2166ac",                         // k = 7
	"b2182bd6604df4a582fddbc7d1e5f092c5de4393c32166ac",                   // k = 8
	"b2182bd6604df4a582fddbc7f7f7f7d1e5f092c5de4393c32166ac",             // k = 9
	"67001fb2182bd6604df4a582fddbc7d1e5f092c5de4393c32166ac053061",       // k = 10
	"67001fb2182bd6604df4a582fddbc7f7f7f7d1e5f092c5de4393c32166ac053061", // k = 11
)

// SchemeRdGy is the ColorBrewer "RdGy" family, running red to grey.
// Indexable by class count k for k = 3 to 11.
var SchemeRdGy = family(
	"ef8a62ffffff999999",                                                 // k = 3
	"ca0020f4a582bababa404040",                                           // k = 4
	"ca0020f4a582ffffffbababa404040",                                     // k = 5
	"b2182bef8a62fddbc7e0e0e09999994d4d4d",                               // k = 6
	"b2182bef8a62fddbc7ffffffe0e0e09999994d4d4d",                         // k = 7
	"b2182bd6604df4a582fddbc7e0e0e0bababa8787874d4d4d",                   // k = 8
	"b2182bd6604df4a582fddbc7ffffffe0e0e0bababa8787874d4d4d",             // k = 9
	"67001fb2182bd6604df4a582fddbc7e0e0e0bababa8787874d4d4d1a1a1a",       // k = 10
	"67001fb2182bd6604df4a582fddbc7ffffffe0e0e0bababa8787874d4d4d1a1a1a", // k = 11
)

// SchemeRdYlBu is the ColorBrewer "RdYlBu" family, running red to yellow to blue.
// Indexable by class count k for k = 3 to 11.
var SchemeRdYlBu = family(
	"fc8d59ffffbf91bfdb",                                                 // k = 3
	"d7191cfdae61abd9e92c7bb6",                                           // k = 4
	"d7191cfdae61ffffbfabd9e92c7bb6",                                     // k = 5
	"d73027fc8d59fee090e0f3f891bfdb4575b4",                               // k = 6
	"d73027fc8d59fee090ffffbfe0f3f891bfdb4575b4",                         // k = 7
	"d73027f46d43fdae61fee090e0f3f8abd9e974add14575b4",                   // k = 8
	"d73027f46d43fdae61fee090ffffbfe0f3f8abd9e974add14575b4",             // k = 9
	"a50026d73027f46d43fdae61fee090e0f3f8abd9e974add14575b4313695",       // k = 10
	"a50026d73027f46d43fdae61fee090ffffbfe0f3f8abd9e974add14575b4313695", // k = 11
)

// SchemeRdYlGn is the ColorBrewer "RdYlGn" family, running red to yellow to green.
// Indexable by class count k for k = 3 to 11.
var SchemeRdYlGn = family(
	"fc8d59ffffbf91cf60",                                                 // k = 3
	"d7191cfdae61a6d96a1a9641",                                           // k = 4
	"d7191cfdae61ffffbfa6d96a1a9641",                                     // k = 5
	"d73027fc8d59fee08bd9ef8b91cf601a9850",                               // k = 6
	"d73027fc8d59fee08bffffbfd9ef8b91cf601a9850",                         // k = 7
	"d73027f46d43fdae61fee08bd9ef8ba6d96a66bd631a9850",                   // k = 8
	"d73027f46d43fdae61fee08bffffbfd9ef8ba6d96a66bd631a9850",             // k = 9
	"a50026d73027f46d43fdae61fee08bd9ef8ba6d96a66bd631a9850006837",       // k = 10
	"a50026d73027f46d43fdae61fee08bffffbfd9ef8ba6d96a66bd631a9850006837", // k = 11
)

// SchemeSpectral is the ColorBrewer "Spectral" family, running the full red-to-blue spectral sweep.
// Indexable by class count k for k = 3 to 11.
var SchemeSpectral = family(
	"fc8d59ffffbf99d594",                                                 // k = 3
	"d7191cfdae61abdda42b83ba",                                           // k = 4
	"d7191cfdae61ffffbfabdda42b83ba",                                     // k = 5
	"d53e4ffc8d59fee08be6f59899d5943288bd",                               // k = 6
	"d53e4ffc8d59fee08bffffbfe6f59899d5943288bd",                         // k = 7
	"d53e4ff46d43fdae61fee08be6f598abdda466c2a53288bd",                   // k = 8
	"d53e4ff46d43fdae61fee08bffffbfe6f598abdda466c2a53288bd",             // k = 9
	"9e0142d53e4ff46d43fdae61fee08be6f598abdda466c2a53288bd5e4fa2",       // k = 10
	"9e0142d53e4ff46d43fdae61fee08bffffbfe6f598abdda466c2a53288bd5e4fa2", // k = 11
)

// Sequential families. Each is published for k = 3 to 9 and runs light to dark,
// so the darkest class is the largest value.

// SchemeBuGn is the ColorBrewer "BuGn" family, running blue to green.
// Indexable by class count k for k = 3 to 9.
var SchemeBuGn = family(
	"e5f5f999d8c92ca25f",                                     // k = 3
	"edf8fbb2e2e266c2a4238b45",                               // k = 4
	"edf8fbb2e2e266c2a42ca25f006d2c",                         // k = 5
	"edf8fbccece699d8c966c2a42ca25f006d2c",                   // k = 6
	"edf8fbccece699d8c966c2a441ae76238b45005824",             // k = 7
	"f7fcfde5f5f9ccece699d8c966c2a441ae76238b45005824",       // k = 8
	"f7fcfde5f5f9ccece699d8c966c2a441ae76238b45006d2c00441b", // k = 9
)

// SchemeBuPu is the ColorBrewer "BuPu" family, running blue to purple.
// Indexable by class count k for k = 3 to 9.
var SchemeBuPu = family(
	"e0ecf49ebcda8856a7",                                     // k = 3
	"edf8fbb3cde38c96c688419d",                               // k = 4
	"edf8fbb3cde38c96c68856a7810f7c",                         // k = 5
	"edf8fbbfd3e69ebcda8c96c68856a7810f7c",                   // k = 6
	"edf8fbbfd3e69ebcda8c96c68c6bb188419d6e016b",             // k = 7
	"f7fcfde0ecf4bfd3e69ebcda8c96c68c6bb188419d6e016b",       // k = 8
	"f7fcfde0ecf4bfd3e69ebcda8c96c68c6bb188419d810f7c4d004b", // k = 9
)

// SchemeGnBu is the ColorBrewer "GnBu" family, running green to blue.
// Indexable by class count k for k = 3 to 9.
var SchemeGnBu = family(
	"e0f3dba8ddb543a2ca",                                     // k = 3
	"f0f9e8bae4bc7bccc42b8cbe",                               // k = 4
	"f0f9e8bae4bc7bccc443a2ca0868ac",                         // k = 5
	"f0f9e8ccebc5a8ddb57bccc443a2ca0868ac",                   // k = 6
	"f0f9e8ccebc5a8ddb57bccc44eb3d32b8cbe08589e",             // k = 7
	"f7fcf0e0f3dbccebc5a8ddb57bccc44eb3d32b8cbe08589e",       // k = 8
	"f7fcf0e0f3dbccebc5a8ddb57bccc44eb3d32b8cbe0868ac084081", // k = 9
)

// SchemeOrRd is the ColorBrewer "OrRd" family, running orange to red.
// Indexable by class count k for k = 3 to 9.
var SchemeOrRd = family(
	"fee8c8fdbb84e34a33",                                     // k = 3
	"fef0d9fdcc8afc8d59d7301f",                               // k = 4
	"fef0d9fdcc8afc8d59e34a33b30000",                         // k = 5
	"fef0d9fdd49efdbb84fc8d59e34a33b30000",                   // k = 6
	"fef0d9fdd49efdbb84fc8d59ef6548d7301f990000",             // k = 7
	"fff7ecfee8c8fdd49efdbb84fc8d59ef6548d7301f990000",       // k = 8
	"fff7ecfee8c8fdd49efdbb84fc8d59ef6548d7301fb300007f0000", // k = 9
)

// SchemePuBuGn is the ColorBrewer "PuBuGn" family, running purple to blue to green.
// Indexable by class count k for k = 3 to 9.
var SchemePuBuGn = family(
	"ece2f0a6bddb1c9099",                                     // k = 3
	"f6eff7bdc9e167a9cf02818a",                               // k = 4
	"f6eff7bdc9e167a9cf1c9099016c59",                         // k = 5
	"f6eff7d0d1e6a6bddb67a9cf1c9099016c59",                   // k = 6
	"f6eff7d0d1e6a6bddb67a9cf3690c002818a016450",             // k = 7
	"fff7fbece2f0d0d1e6a6bddb67a9cf3690c002818a016450",       // k = 8
	"fff7fbece2f0d0d1e6a6bddb67a9cf3690c002818a016c59014636", // k = 9
)

// SchemePuBu is the ColorBrewer "PuBu" family, running purple to blue.
// Indexable by class count k for k = 3 to 9.
var SchemePuBu = family(
	"ece7f2a6bddb2b8cbe",                                     // k = 3
	"f1eef6bdc9e174a9cf0570b0",                               // k = 4
	"f1eef6bdc9e174a9cf2b8cbe045a8d",                         // k = 5
	"f1eef6d0d1e6a6bddb74a9cf2b8cbe045a8d",                   // k = 6
	"f1eef6d0d1e6a6bddb74a9cf3690c00570b0034e7b",             // k = 7
	"fff7fbece7f2d0d1e6a6bddb74a9cf3690c00570b0034e7b",       // k = 8
	"fff7fbece7f2d0d1e6a6bddb74a9cf3690c00570b0045a8d023858", // k = 9
)

// SchemePuRd is the ColorBrewer "PuRd" family, running purple to red.
// Indexable by class count k for k = 3 to 9.
var SchemePuRd = family(
	"e7e1efc994c7dd1c77",                                     // k = 3
	"f1eef6d7b5d8df65b0ce1256",                               // k = 4
	"f1eef6d7b5d8df65b0dd1c77980043",                         // k = 5
	"f1eef6d4b9dac994c7df65b0dd1c77980043",                   // k = 6
	"f1eef6d4b9dac994c7df65b0e7298ace125691003f",             // k = 7
	"f7f4f9e7e1efd4b9dac994c7df65b0e7298ace125691003f",       // k = 8
	"f7f4f9e7e1efd4b9dac994c7df65b0e7298ace125698004367001f", // k = 9
)

// SchemeRdPu is the ColorBrewer "RdPu" family, running red to purple.
// Indexable by class count k for k = 3 to 9.
var SchemeRdPu = family(
	"fde0ddfa9fb5c51b8a",                                     // k = 3
	"feebe2fbb4b9f768a1ae017e",                               // k = 4
	"feebe2fbb4b9f768a1c51b8a7a0177",                         // k = 5
	"feebe2fcc5c0fa9fb5f768a1c51b8a7a0177",                   // k = 6
	"feebe2fcc5c0fa9fb5f768a1dd3497ae017e7a0177",             // k = 7
	"fff7f3fde0ddfcc5c0fa9fb5f768a1dd3497ae017e7a0177",       // k = 8
	"fff7f3fde0ddfcc5c0fa9fb5f768a1dd3497ae017e7a017749006a", // k = 9
)

// SchemeYlGnBu is the ColorBrewer "YlGnBu" family, running yellow to green to blue.
// Indexable by class count k for k = 3 to 9.
var SchemeYlGnBu = family(
	"edf8b17fcdbb2c7fb8",                                     // k = 3
	"ffffcca1dab441b6c4225ea8",                               // k = 4
	"ffffcca1dab441b6c42c7fb8253494",                         // k = 5
	"ffffccc7e9b47fcdbb41b6c42c7fb8253494",                   // k = 6
	"ffffccc7e9b47fcdbb41b6c41d91c0225ea80c2c84",             // k = 7
	"ffffd9edf8b1c7e9b47fcdbb41b6c41d91c0225ea80c2c84",       // k = 8
	"ffffd9edf8b1c7e9b47fcdbb41b6c41d91c0225ea8253494081d58", // k = 9
)

// SchemeYlGn is the ColorBrewer "YlGn" family, running yellow to green.
// Indexable by class count k for k = 3 to 9.
var SchemeYlGn = family(
	"f7fcb9addd8e31a354",                                     // k = 3
	"ffffccc2e69978c679238443",                               // k = 4
	"ffffccc2e69978c67931a354006837",                         // k = 5
	"ffffccd9f0a3addd8e78c67931a354006837",                   // k = 6
	"ffffccd9f0a3addd8e78c67941ab5d238443005a32",             // k = 7
	"ffffe5f7fcb9d9f0a3addd8e78c67941ab5d238443005a32",       // k = 8
	"ffffe5f7fcb9d9f0a3addd8e78c67941ab5d238443006837004529", // k = 9
)

// SchemeYlOrBr is the ColorBrewer "YlOrBr" family, running yellow to orange to brown.
// Indexable by class count k for k = 3 to 9.
var SchemeYlOrBr = family(
	"fff7bcfec44fd95f0e",                                     // k = 3
	"ffffd4fed98efe9929cc4c02",                               // k = 4
	"ffffd4fed98efe9929d95f0e993404",                         // k = 5
	"ffffd4fee391fec44ffe9929d95f0e993404",                   // k = 6
	"ffffd4fee391fec44ffe9929ec7014cc4c028c2d04",             // k = 7
	"ffffe5fff7bcfee391fec44ffe9929ec7014cc4c028c2d04",       // k = 8
	"ffffe5fff7bcfee391fec44ffe9929ec7014cc4c02993404662506", // k = 9
)

// SchemeYlOrRd is the ColorBrewer "YlOrRd" family, running yellow to orange to red.
// Indexable by class count k for k = 3 to 9.
var SchemeYlOrRd = family(
	"ffeda0feb24cf03b20",                                     // k = 3
	"ffffb2fecc5cfd8d3ce31a1c",                               // k = 4
	"ffffb2fecc5cfd8d3cf03b20bd0026",                         // k = 5
	"ffffb2fed976feb24cfd8d3cf03b20bd0026",                   // k = 6
	"ffffb2fed976feb24cfd8d3cfc4e2ae31a1cb10026",             // k = 7
	"ffffccffeda0fed976feb24cfd8d3cfc4e2ae31a1cb10026",       // k = 8
	"ffffccffeda0fed976feb24cfd8d3cfc4e2ae31a1cbd0026800026", // k = 9
)

// SchemeBlues is the ColorBrewer "Blues" family, running white to dark blue.
// Indexable by class count k for k = 3 to 9.
var SchemeBlues = family(
	"deebf79ecae13182bd",                                     // k = 3
	"eff3ffbdd7e76baed62171b5",                               // k = 4
	"eff3ffbdd7e76baed63182bd08519c",                         // k = 5
	"eff3ffc6dbef9ecae16baed63182bd08519c",                   // k = 6
	"eff3ffc6dbef9ecae16baed64292c62171b5084594",             // k = 7
	"f7fbffdeebf7c6dbef9ecae16baed64292c62171b5084594",       // k = 8
	"f7fbffdeebf7c6dbef9ecae16baed64292c62171b508519c08306b", // k = 9
)

// SchemeGreens is the ColorBrewer "Greens" family, running white to dark green.
// Indexable by class count k for k = 3 to 9.
var SchemeGreens = family(
	"e5f5e0a1d99b31a354",                                     // k = 3
	"edf8e9bae4b374c476238b45",                               // k = 4
	"edf8e9bae4b374c47631a354006d2c",                         // k = 5
	"edf8e9c7e9c0a1d99b74c47631a354006d2c",                   // k = 6
	"edf8e9c7e9c0a1d99b74c47641ab5d238b45005a32",             // k = 7
	"f7fcf5e5f5e0c7e9c0a1d99b74c47641ab5d238b45005a32",       // k = 8
	"f7fcf5e5f5e0c7e9c0a1d99b74c47641ab5d238b45006d2c00441b", // k = 9
)

// SchemeGreys is the ColorBrewer "Greys" family, running white to black.
// Indexable by class count k for k = 3 to 9.
var SchemeGreys = family(
	"f0f0f0bdbdbd636363",                                     // k = 3
	"f7f7f7cccccc969696525252",                               // k = 4
	"f7f7f7cccccc969696636363252525",                         // k = 5
	"f7f7f7d9d9d9bdbdbd969696636363252525",                   // k = 6
	"f7f7f7d9d9d9bdbdbd969696737373525252252525",             // k = 7
	"fffffff0f0f0d9d9d9bdbdbd969696737373525252252525",       // k = 8
	"fffffff0f0f0d9d9d9bdbdbd969696737373525252252525000000", // k = 9
)

// SchemePurples is the ColorBrewer "Purples" family, running white to dark purple.
// Indexable by class count k for k = 3 to 9.
var SchemePurples = family(
	"efedf5bcbddc756bb1",                                     // k = 3
	"f2f0f7cbc9e29e9ac86a51a3",                               // k = 4
	"f2f0f7cbc9e29e9ac8756bb154278f",                         // k = 5
	"f2f0f7dadaebbcbddc9e9ac8756bb154278f",                   // k = 6
	"f2f0f7dadaebbcbddc9e9ac8807dba6a51a34a1486",             // k = 7
	"fcfbfdefedf5dadaebbcbddc9e9ac8807dba6a51a34a1486",       // k = 8
	"fcfbfdefedf5dadaebbcbddc9e9ac8807dba6a51a354278f3f007d", // k = 9
)

// SchemeReds is the ColorBrewer "Reds" family, running white to dark red.
// Indexable by class count k for k = 3 to 9.
var SchemeReds = family(
	"fee0d2fc9272de2d26",                                     // k = 3
	"fee5d9fcae91fb6a4acb181d",                               // k = 4
	"fee5d9fcae91fb6a4ade2d26a50f15",                         // k = 5
	"fee5d9fcbba1fc9272fb6a4ade2d26a50f15",                   // k = 6
	"fee5d9fcbba1fc9272fb6a4aef3b2ccb181d99000d",             // k = 7
	"fff5f0fee0d2fcbba1fc9272fb6a4aef3b2ccb181d99000d",       // k = 8
	"fff5f0fee0d2fcbba1fc9272fb6a4aef3b2ccb181da50f1567000d", // k = 9
)

// SchemeOranges is the ColorBrewer "Oranges" family, running white to dark orange.
// Indexable by class count k for k = 3 to 9.
var SchemeOranges = family(
	"fee6cefdae6be6550d",                                     // k = 3
	"feeddefdbe85fd8d3cd94701",                               // k = 4
	"feeddefdbe85fd8d3ce6550da63603",                         // k = 5
	"feeddefdd0a2fdae6bfd8d3ce6550da63603",                   // k = 6
	"feeddefdd0a2fdae6bfd8d3cf16913d948018c2d04",             // k = 7
	"fff5ebfee6cefdd0a2fdae6bfd8d3cf16913d948018c2d04",       // k = 8
	"fff5ebfee6cefdd0a2fdae6bfd8d3cf16913d94801a636037f2704", // k = 9
)

// The matplotlib ramps are published as 256 sampled colors rather than as a
// handful of stops, because they were computed to be perceptually uniform and a
// spline through a few stops would not preserve that. See [InterpolateViridis].

// tableViridis holds the 256 sampled colors of Viridis, which is matplotlib's default, and the reference for perceptually uniform: it is
// monotone in lightness, colorblind-friendly, and prints legibly in grayscale.
var tableViridis = colors("44015444025645045745055946075a46085c460a5d460b5e470d60470e6147106347116447136548146748166848176948186a481a6c481b6d481c6e481d6f481f70482071482173482374482475482576482677482878482979472a7a472c7a472d7b472e7c472f7d46307e46327e46337f463480453581453781453882443983443a83443b84433d84433e85423f854240864241864142874144874045884046883f47883f48893e49893e4a893e4c8a3d4d8a3d4e8a3c4f8a3c508b3b518b3b528b3a538b3a548c39558c39568c38588c38598c375a8c375b8d365c8d365d8d355e8d355f8d34608d34618d33628d33638d32648e32658e31668e31678e31688e30698e306a8e2f6b8e2f6c8e2e6d8e2e6e8e2e6f8e2d708e2d718e2c718e2c728e2c738e2b748e2b758e2a768e2a778e2a788e29798e297a8e297b8e287c8e287d8e277e8e277f8e27808e26818e26828e26828e25838e25848e25858e24868e24878e23888e23898e238a8d228b8d228c8d228d8d218e8d218f8d21908d21918c20928c20928c20938c1f948c1f958b1f968b1f978b1f988b1f998a1f9a8a1e9b8a1e9c891e9d891f9e891f9f881fa0881fa1881fa1871fa28720a38620a48621a58521a68522a78522a88423a98324aa8325ab8225ac8226ad8127ad8128ae8029af7f2ab07f2cb17e2db27d2eb37c2fb47c31b57b32b67a34b67935b77937b87838b9773aba763bbb753dbc743fbc7340bd7242be7144bf7046c06f48c16e4ac16d4cc26c4ec36b50c46a52c56954c56856c66758c7655ac8645cc8635ec96260ca6063cb5f65cb5e67cc5c69cd5b6ccd5a6ece5870cf5773d05675d05477d1537ad1517cd2507fd34e81d34d84d44b86d54989d5488bd6468ed64590d74393d74195d84098d83e9bd93c9dd93ba0da39a2da37a5db36a8db34aadc32addc30b0dd2fb2dd2db5de2bb8de29bade28bddf26c0df25c2df23c5e021c8e020cae11fcde11dd0e11cd2e21bd5e21ad8e219dae319dde318dfe318e2e418e5e419e7e419eae51aece51befe51cf1e51df4e61ef6e620f8e621fbe723fde725")

// tableMagma holds the 256 sampled colors of Magma, which is one of matplotlib's perceptually uniform ramps, black through magenta to
// pale yellow.
var tableMagma = colors("00000401000501010601010802010902020b02020d03030f03031204041405041606051806051a07061c08071e0907200a08220b09240c09260d0a290e0b2b100b2d110c2f120d31130d34140e36150e38160f3b180f3d19103f1a10421c10441d11471e114920114b21114e22115024125325125527125829115a2a115c2c115f2d11612f116331116533106734106936106b38106c390f6e3b0f703d0f713f0f72400f74420f75440f764510774710784910784a10794c117a4e117b4f127b51127c52137c54137d56147d57157e59157e5a167e5c167f5d177f5f187f601880621980641a80651a80671b80681c816a1c816b1d816d1d816e1e81701f81721f817320817521817621817822817922827b23827c23827e24828025828125818326818426818627818827818928818b29818c29818e2a81902a81912b81932b80942c80962c80982d80992d809b2e7f9c2e7f9e2f7fa02f7fa1307ea3307ea5317ea6317da8327daa337dab337cad347cae347bb0357bb2357bb3367ab5367ab73779b83779ba3878bc3978bd3977bf3a77c03a76c23b75c43c75c53c74c73d73c83e73ca3e72cc3f71cd4071cf4070d0416fd2426fd3436ed5446dd6456cd8456cd9466bdb476adc4869de4968df4a68e04c67e24d66e34e65e44f64e55064e75263e85362e95462ea5661eb5760ec5860ed5a5fee5b5eef5d5ef05f5ef1605df2625df2645cf3655cf4675cf4695cf56b5cf66c5cf66e5cf7705cf7725cf8745cf8765cf9785df9795df97b5dfa7d5efa7f5efa815ffb835ffb8560fb8761fc8961fc8a62fc8c63fc8e64fc9065fd9266fd9467fd9668fd9869fd9a6afd9b6bfe9d6cfe9f6dfea16efea36ffea571fea772fea973feaa74feac76feae77feb078feb27afeb47bfeb67cfeb77efeb97ffebb81febd82febf84fec185fec287fec488fec68afec88cfeca8dfecc8ffecd90fecf92fed194fed395fed597fed799fed89afdda9cfddc9efddea0fde0a1fde2a3fde3a5fde5a7fde7a9fde9aafdebacfcecaefceeb0fcf0b2fcf2b4fcf4b6fcf6b8fcf7b9fcf9bbfcfbbdfcfdbf")

// tableInferno holds the 256 sampled colors of Inferno, which is one of matplotlib's perceptually uniform ramps, black through red to pale
// yellow — the most saturated of the four.
var tableInferno = colors("00000401000501010601010802010a02020c02020e03021004031204031405041706041907051b08051d09061f0a07220b07240c08260d08290e092b10092d110a30120a32140b34150b37160b39180c3c190c3e1b0c411c0c431e0c451f0c48210c4a230c4c240c4f260c51280b53290b552b0b572d0b592f0a5b310a5c320a5e340a5f3609613809623909633b09643d09653e0966400a67420a68440a68450a69470b6a490b6a4a0c6b4c0c6b4d0d6c4f0d6c510e6c520e6d540f6d550f6d57106e59106e5a116e5c126e5d126e5f136e61136e62146e64156e65156e67166e69166e6a176e6c186e6d186e6f196e71196e721a6e741a6e751b6e771c6d781c6d7a1d6d7c1d6d7d1e6d7f1e6c801f6c82206c84206b85216b87216b88226a8a226a8c23698d23698f24699025689225689326679526679727669827669a28659b29649d29649f2a63a02a63a22b62a32c61a52c60a62d60a82e5fa92e5eab2f5ead305dae305cb0315bb1325ab3325ab43359b63458b73557b93556ba3655bc3754bd3853bf3952c03a51c13a50c33b4fc43c4ec63d4dc73e4cc83f4bca404acb4149cc4248ce4347cf4446d04545d24644d34743d44842d54a41d74b3fd84c3ed94d3dda4e3cdb503bdd513ade5238df5337e05536e15635e25734e35933e45a31e55c30e65d2fe75e2ee8602de9612bea632aeb6429eb6628ec6726ed6925ee6a24ef6c23ef6e21f06f20f1711ff1731df2741cf3761bf37819f47918f57b17f57d15f67e14f68013f78212f78410f8850ff8870ef8890cf98b0bf98c0af98e09fa9008fa9207fa9407fb9606fb9706fb9906fb9b06fb9d07fc9f07fca108fca309fca50afca60cfca80dfcaa0ffcac11fcae12fcb014fcb216fcb418fbb61afbb81dfbba1ffbbc21fbbe23fac026fac228fac42afac62df9c72ff9c932f9cb35f8cd37f8cf3af7d13df7d340f6d543f6d746f5d949f5db4cf4dd4ff4df53f4e156f3e35af3e55df2e661f2e865f2ea69f1ec6df1ed71f1ef75f1f179f2f27df2f482f3f586f3f68af4f88ef5f992f6fa96f8fb9af9fc9dfafda1fcffa4")

// tablePlasma holds the 256 sampled colors of Plasma, which is one of matplotlib's perceptually uniform ramps, dark blue through magenta
// to yellow.
var tablePlasma = colors("0d088710078813078916078a19068c1b068d1d068e20068f2206902406912605912805922a05932c05942e05952f059631059733059735049837049938049a3a049a3c049b3e049c3f049c41049d43039e44039e46039f48039f4903a04b03a14c02a14e02a25002a25102a35302a35502a45601a45801a45901a55b01a55c01a65e01a66001a66100a76300a76400a76600a76700a86900a86a00a86c00a86e00a86f00a87100a87201a87401a87501a87701a87801a87a02a87b02a87d03a87e03a88004a88104a78305a78405a78606a68707a68808a68a09a58b0aa58d0ba58e0ca48f0da4910ea3920fa39410a29511a19613a19814a099159f9a169f9c179e9d189d9e199da01a9ca11b9ba21d9aa31e9aa51f99a62098a72197a82296aa2395ab2494ac2694ad2793ae2892b02991b12a90b22b8fb32c8eb42e8db52f8cb6308bb7318ab83289ba3388bb3488bc3587bd3786be3885bf3984c03a83c13b82c23c81c33d80c43e7fc5407ec6417dc7427cc8437bc9447aca457acb4679cc4778cc4977cd4a76ce4b75cf4c74d04d73d14e72d24f71d35171d45270d5536fd5546ed6556dd7566cd8576bd9586ada5a6ada5b69db5c68dc5d67dd5e66de5f65de6164df6263e06363e16462e26561e26660e3685fe4695ee56a5de56b5de66c5ce76e5be76f5ae87059e97158e97257ea7457eb7556eb7655ec7754ed7953ed7a52ee7b51ef7c51ef7e50f07f4ff0804ef1814df1834cf2844bf3854bf3874af48849f48948f58b47f58c46f68d45f68f44f79044f79143f79342f89441f89540f9973ff9983ef99a3efa9b3dfa9c3cfa9e3bfb9f3afba139fba238fca338fca537fca636fca835fca934fdab33fdac33fdae32fdaf31fdb130fdb22ffdb42ffdb52efeb72dfeb82cfeba2cfebb2bfebd2afebe2afec029fdc229fdc328fdc527fdc627fdc827fdca26fdcb26fccd25fcce25fcd025fcd225fbd324fbd524fbd724fad824fada24f9dc24f9dd25f8df25f8e125f7e225f7e425f6e626f6e826f5e926f5eb27f4ed27f3ee27f3f027f2f227f1f426f1f525f0f724f0f921")
