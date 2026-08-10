'use strict';
// Upstream (Node) parity runner for pdfkit@0.15.1.
//
// Reads one JSON object per line on stdin, writes exactly one JSON object per
// line on stdout, in the same order. A throw is reported as {"ok":false} and the
// loop keeps going; the process never exits early on a failing case.
//
// The comparable artefact is the finished PDF, returned as lowercase hex. The
// harness (Go side) parses it and derives the canonical structural description,
// so exactly one parser is applied to both sides' output.
//
// Determinism pins (see COVERAGE.md):
//   * info.CreationDate is fixed to 2026-01-02T15:04:05Z, which also fixes
//     pdfkit's file id (an MD5 over the /Info dictionary).
//   * info.Producer and info.Creator are pinned to "parity" on both sides.
//   * compress:false, so page content streams are literal (the Go port never
//     compresses them).
//   * bufferPages:true and autoFirstPage:false, so page creation is explicit.

const fs = require('fs');
const path = require('path');
const PDFDocument = require('pdfkit');

const ROOT = process.env.PARITY_ROOT || process.cwd();
const FIXED_DATE = new Date(Date.UTC(2026, 0, 2, 15, 4, 5));

const PAGE_SIZES = {
  A3: [841.89, 1190.55],
  A4: [595.28, 841.89],
  A5: [419.53, 595.28],
  LETTER: [612, 792],
  LEGAL: [612, 1008],
  TABLOID: [792, 1224],
};

const STANDARD_FONTS = new Set([
  'Helvetica', 'Helvetica-Bold', 'Helvetica-Oblique', 'Helvetica-BoldOblique',
  'Times-Roman', 'Times-Bold', 'Times-Italic', 'Times-BoldItalic',
  'Courier', 'Courier-Bold', 'Courier-Oblique', 'Courier-BoldOblique',
  'Symbol', 'ZapfDingbats',
]);

function fixture(rel) {
  if (/^[a-zA-Z]+:\/\//.test(rel)) throw new Error('remote paths are not allowed in the parity harness');
  const p = path.isAbsolute(rel) ? rel : path.join(ROOT, rel);
  if (!p.startsWith(ROOT + path.sep) && p !== ROOT) throw new Error('fixture path escapes the harness root: ' + rel);
  return fs.readFileSync(p);
}

function sizeOf(spec) {
  if (Array.isArray(spec)) return [num(spec[0]), num(spec[1])];
  const s = PAGE_SIZES[String(spec).toUpperCase()];
  if (!s) throw new Error('unknown page size: ' + spec);
  return s;
}

function num(v) {
  if (typeof v !== 'number' || !Number.isFinite(v)) throw new Error('expected a finite number, got ' + JSON.stringify(v));
  return v;
}

// rgb takes 0..255 components, which is what pdfkit's _normalizeColor expects
// for a 3-element array. cmyk takes 0..100, likewise.
function rgb(a) {
  if (!Array.isArray(a) || a.length !== 3) throw new Error('rgb expects [r,g,b] 0..255');
  return [num(a[0]), num(a[1]), num(a[2])];
}
function cmyk(a) {
  if (!Array.isArray(a) || a.length !== 4) throw new Error('cmyk expects [c,m,y,k] 0..100');
  return [num(a[0]), num(a[1]), num(a[2]), num(a[3])];
}

const ALIGN = { left: 'left', center: 'center', right: 'right', justify: 'justify' };

// Emits a raw content-stream fragment for a tiling-pattern cell. pdfkit's
// pattern() takes the cell as a literal operator string, so the neutral cell ops
// are rendered here; the Go port instead hands a detached Page to a callback.
function cellStream(ops) {
  const out = [];
  const n = (v) => {
    const r = Math.round(num(v) * 1e6) / 1e6;
    return String(r);
  };
  for (const op of ops || []) {
    const a = op.args || [];
    switch (op.op) {
      case 'rect': out.push(`${n(a[0])} ${n(a[1])} ${n(a[2])} ${n(a[3])} re`); break;
      case 'moveTo': out.push(`${n(a[0])} ${n(a[1])} m`); break;
      case 'lineTo': out.push(`${n(a[0])} ${n(a[1])} l`); break;
      case 'closePath': out.push('h'); break;
      case 'fill': out.push('f'); break;
      case 'stroke': out.push('S'); break;
      case 'lineWidth': out.push(`${n(a[0])} w`); break;
      case 'fillColor': { const c = rgb(a[0]); out.push(`${n(c[0] / 255)} ${n(c[1] / 255)} ${n(c[2] / 255)} rg`); break; }
      case 'strokeColor': { const c = rgb(a[0]); out.push(`${n(c[0] / 255)} ${n(c[1] / 255)} ${n(c[2] / 255)} RG`); break; }
      default: throw new Error('a pdfkit tiling pattern is a raw content-stream string with no font or XObject resources, so it cannot express: ' + op.op);
    }
  }
  return out.join('\n');
}

function makeGradient(doc, op) {
  const a = op.args;
  let g;
  if (op.op === 'linearGradient') {
    g = doc.linearGradient(num(a[0]), num(a[1]), num(a[2]), num(a[3]));
  } else {
    g = doc.radialGradient(num(a[0]), num(a[1]), num(a[2]), num(a[3]), num(a[4]), num(a[5]));
  }
  const stops = op.stops || [];
  if (stops.length < 2) throw new Error('a gradient needs at least two stops');
  for (const s of stops) g.stop(num(s.offset), rgb(s.color), s.opacity === undefined ? undefined : num(s.opacity));
  return g;
}

function initForm(doc) {
  if (!doc._formInitialised) {
    doc.initForm();
    doc._formInitialised = true;
  }
}

function applyOps(doc, spec, ops) {
  for (const op of ops) {
    const a = op.args || [];
    switch (op.op) {
      // --- text -------------------------------------------------------------
      case 'setFont': {
        const name = String(a[0]);
        if (!STANDARD_FONTS.has(name)) throw new Error('not one of the standard 14 fonts: ' + name);
        doc.font(name).fontSize(num(a[1]));
        break;
      }
      case 'text':
        doc.text(String(a[2]), num(a[0]), num(a[1]), { lineBreak: false });
        break;
      case 'textAligned':
        doc.text(String(a[3]), num(a[0]), num(a[1]), {
          width: num(a[2]), align: ALIGN[a[4]] || 'left', lineBreak: false,
        });
        break;
      case 'textBox':
        doc.text(String(a[4]), num(a[0]), num(a[1]), {
          width: num(a[2]), lineGap: num(a[3]), align: ALIGN[a[5]] || 'left',
        });
        break;
      case 'textDecorated':
        doc.text(String(a[3]), num(a[0]), num(a[1]), {
          width: num(a[2]), lineBreak: false, underline: !!op.underline, strike: !!op.strike,
        });
        break;
      case 'textCharSpacing':
        doc.text(String(a[3]), num(a[0]), num(a[1]), { lineBreak: false, characterSpacing: num(a[2]) });
        break;
      case 'textFlow': // relies on the layout cursor: no x/y at all
        doc.text(String(a[0]));
        break;
      case 'moveDown': doc.moveDown(num(a[0])); break;
      case 'moveUp': doc.moveUp(num(a[0])); break;
      case 'list': doc.list(a[0].map(String), num(a[1]), num(a[2])); break;
      case 'lineGap': doc.lineGap(num(a[0])); break;

      // --- vector -----------------------------------------------------------
      case 'moveTo': doc.moveTo(num(a[0]), num(a[1])); break;
      case 'lineTo': doc.lineTo(num(a[0]), num(a[1])); break;
      case 'curveTo': doc.bezierCurveTo(num(a[0]), num(a[1]), num(a[2]), num(a[3]), num(a[4]), num(a[5])); break;
      case 'quadraticCurveTo': doc.quadraticCurveTo(num(a[0]), num(a[1]), num(a[2]), num(a[3])); break;
      case 'rect': doc.rect(num(a[0]), num(a[1]), num(a[2]), num(a[3])); break;
      case 'roundedRect': doc.roundedRect(num(a[0]), num(a[1]), num(a[2]), num(a[3]), num(a[4])); break;
      case 'circle': doc.circle(num(a[0]), num(a[1]), num(a[2])); break;
      case 'ellipse': doc.ellipse(num(a[0]), num(a[1]), num(a[2]), num(a[3])); break;
      case 'polygon': doc.polygon(...chunk2(a)); break;
      case 'closePath': doc.closePath(); break;
      case 'svgPath': doc.path(String(a[0])); break;
      case 'fill': doc.fill(); break;
      case 'fillEvenOdd': doc.fill('even-odd'); break;
      case 'stroke': doc.stroke(); break;
      case 'fillStroke': doc.fillAndStroke(); break;
      case 'fillColor': doc.fillColor(rgb(a[0])); break;
      case 'strokeColor': doc.strokeColor(rgb(a[0])); break;
      case 'fillCMYK': doc.fillColor(cmyk(a[0])); break;
      case 'strokeCMYK': doc.strokeColor(cmyk(a[0])); break;
      case 'lineWidth': doc.lineWidth(num(a[0])); break;
      case 'lineCap': doc.lineCap(num(a[0])); break;
      case 'lineJoin': doc.lineJoin(num(a[0])); break;
      case 'miterLimit': doc.miterLimit(num(a[0])); break;
      case 'dash': doc.dash(a.slice(1).map(num), { phase: num(a[0]) }); break;
      case 'undash': doc.undash(); break;

      // --- state / transforms -----------------------------------------------
      case 'save': doc.save(); break;
      case 'restore': doc.restore(); break;
      case 'translate': doc.translate(num(a[0]), num(a[1])); break;
      case 'scale': doc.scale(num(a[0]), num(a[1])); break;
      case 'rotate': doc.rotate(num(a[0]), op.origin ? { origin: op.origin.map(num) } : undefined); break;
      case 'transform': doc.transform(num(a[0]), num(a[1]), num(a[2]), num(a[3]), num(a[4]), num(a[5])); break;
      case 'clip': doc.clip(); break;
      case 'clipEvenOdd': doc.clip('even-odd'); break;

      // --- images -----------------------------------------------------------
      case 'image':
        doc.image(fixture(String(a[0])), num(a[1]), num(a[2]), { width: num(a[3]), height: num(a[4]) });
        break;

      // --- gradients / patterns ---------------------------------------------
      case 'fillGradient': doc.fillColor(makeGradient(doc, op.gradient)); break;
      case 'strokeGradient': doc.strokeColor(makeGradient(doc, op.gradient)); break;
      case 'shade':
        throw new Error('pdfkit has no direct "sh" shading operator (gradients are always pattern fills)');
      case 'fillTilingPattern': {
        // pdfkit tiling patterns are *uncoloured* (PaintType 2): the cell holds
        // geometry only and the colour is supplied when the pattern is selected.
        const w = num(a[0]), h = num(a[1]);
        const pat = doc.pattern([0, 0, w, h], w, h, cellStream(op.cell));
        doc.fillColor([pat, rgb(op.color)]);
        break;
      }

      // --- transparency -----------------------------------------------------
      case 'fillOpacity': doc.fillOpacity(num(a[0])); break;
      case 'strokeOpacity': doc.strokeOpacity(num(a[0])); break;
      case 'opacity': doc.opacity(num(a[0])); break;
      case 'blendMode':
        throw new Error('pdfkit exposes no blend-mode API');

      // --- annotations / links ----------------------------------------------
      case 'linkURI': doc.link(num(a[0]), num(a[1]), num(a[2]), num(a[3]), String(a[4])); break;
      case 'linkToDest': doc.goTo(num(a[0]), num(a[1]), num(a[2]), num(a[3]), String(a[4])); break;
      case 'linkToPage':
        throw new Error('pdfkit has no link-to-page API; internal links go through named destinations');
      case 'namedDest': doc.addNamedDestination(String(a[0]), 'XYZ', num(a[1]), num(a[2]), null); break;
      // note() is the sticky note (/Subtype /Text); textAnnotation() is a
      // free-text annotation (/Subtype /FreeText) despite the name.
      case 'stickyNoteAnnotation': doc.note(num(a[0]), num(a[1]), num(a[2]), num(a[3]), String(a[4])); break;
      case 'freeTextAnnotation': doc.textAnnotation(num(a[0]), num(a[1]), num(a[2]), num(a[3]), String(a[4])); break;
      case 'highlightAnnotation': doc.highlight(num(a[0]), num(a[1]), num(a[2]), num(a[3])); break;
      case 'underlineAnnotation': doc.underline(num(a[0]), num(a[1]), num(a[2]), num(a[3])); break;
      case 'strikeAnnotation': doc.strike(num(a[0]), num(a[1]), num(a[2]), num(a[3])); break;
      case 'lineAnnotation': doc.lineAnnotation(num(a[0]), num(a[1]), num(a[2]), num(a[3])); break;
      case 'rectAnnotation': doc.rectAnnotation(num(a[0]), num(a[1]), num(a[2]), num(a[3])); break;
      case 'ellipseAnnotation': doc.ellipseAnnotation(num(a[0]), num(a[1]), num(a[2]), num(a[3])); break;

      // --- form fields ------------------------------------------------------
      // pdfkit requires an explicit initForm() before any field is added; the
      // port creates the AcroForm implicitly on first use, so the runner calls
      // initForm() lazily to make the two comparable.
      case 'textField':
        initForm(doc);
        doc.formText(String(a[0]), num(a[1]), num(a[2]), num(a[3]), num(a[4]), { value: String(a[5]) });
        break;
      case 'checkbox':
        initForm(doc);
        doc.formCheckbox(String(a[0]), num(a[1]), num(a[2]), num(a[3]), num(a[3]), {
          value: a[4] ? 'Yes' : 'Off',
        });
        break;
      case 'textFieldConfigured':
        initForm(doc);
        doc.formText(String(a[0]), num(a[1]), num(a[2]), num(a[3]), num(a[4]), {
          value: String(a[5]), align: 'center', fontSize: 9, multiline: true,
          required: true, borderColor: '#ff0000', backgroundColor: '#eeeeee',
        });
        break;
      case 'pushButton':
        initForm(doc);
        doc.formPushButton(String(a[0]), num(a[1]), num(a[2]), num(a[3]), num(a[4]), {
          label: String(a[5]),
        });
        break;

      default:
        throw new Error('unknown op: ' + op.op);
    }
  }
}

function chunk2(a) {
  const out = [];
  for (let i = 0; i + 1 < a.length; i += 2) out.push([num(a[i]), num(a[i + 1])]);
  return out;
}

function buildOutline(doc, items, parent) {
  for (const it of items) {
    if (it.page !== undefined) doc.switchToPage(num(it.page));
    const node = (parent || doc.outline).addItem(String(it.title), { expanded: !!it.expanded });
    if (it.children) buildOutline(doc, it.children, node);
  }
}

function buildPDF(spec) {
  const info = { Producer: 'parity', Creator: 'parity', CreationDate: FIXED_DATE };
  const d = spec.doc || {};
  if (d.title !== undefined) info.Title = String(d.title);
  if (d.author !== undefined) info.Author = String(d.author);
  if (d.subject !== undefined) info.Subject = String(d.subject);
  if (d.keywords !== undefined) info.Keywords = String(d.keywords);
  if (d.producer !== undefined) info.Producer = String(d.producer);
  if (d.creator !== undefined) info.Creator = String(d.creator);

  const options = {
    compress: false,
    autoFirstPage: false,
    bufferPages: true,
    info,
  };
  if (d.encrypt) {
    const e = d.encrypt;
    options.userPassword = e.userPassword || undefined;
    options.ownerPassword = e.ownerPassword || undefined;
    options.pdfVersion = e.useAES ? '1.6' : '1.4';
    options.permissions = {
      printing: e.allowPrinting ? 'highResolution' : undefined,
      modifying: !!e.allowModify,
      copying: !!e.allowCopying,
      annotating: !!e.allowAnnotations,
    };
  }

  const doc = new PDFDocument(options);
  const chunks = [];
  doc.on('data', (c) => chunks.push(c));

  const done = new Promise((resolve, reject) => {
    doc.on('end', () => resolve(Buffer.concat(chunks)));
    doc.on('error', reject);
  });

  const pages = spec.pages || [];
  if (pages.length === 0) throw new Error('a case must declare at least one page');
  for (const p of pages) {
    const [w, h] = sizeOf(p.size || 'A4');
    const opts = { size: [w, h] };
    if (p.layout) opts.layout = p.layout;
    if (p.margin !== undefined) opts.margin = num(p.margin);
    if (p.margins) opts.margins = p.margins;
    if (p.rotate !== undefined) throw new Error('pdfkit has no page-rotation option');
    doc.addPage(opts);
    applyOps(doc, p, p.ops || []);
  }
  if (spec.outline) buildOutline(doc, spec.outline, null);
  doc.end();
  return done.then((buf) => buf.toString('hex'));
}

// ---------------------------------------------------------------------------

const rl = require('readline').createInterface({ input: process.stdin, terminal: false });
let queue = Promise.resolve();

rl.on('line', (line) => {
  const text = line.trim();
  if (!text) return;
  queue = queue.then(async () => {
    let req;
    try {
      req = JSON.parse(text);
    } catch (e) {
      process.stdout.write(JSON.stringify({ id: '<unparseable>', ok: false, error: 'bad request line: ' + e.message }) + '\n');
      return;
    }
    try {
      if (req.fn !== 'buildPDF') throw new Error('unknown fn: ' + req.fn);
      if (!req.args || req.args.length === 0) throw new Error('missing document spec argument');
      const hex = await buildPDF(req.args[0]);
      process.stdout.write(JSON.stringify({ id: req.id, ok: true, value: hex }) + '\n');
    } catch (e) {
      process.stdout.write(JSON.stringify({ id: req.id, ok: false, error: String((e && e.message) || e) }) + '\n');
    }
  });
});
