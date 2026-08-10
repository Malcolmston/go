'use strict';
// Upstream parity runner for nodemailer.
//
// Reads one JSON object per line on stdin, writes exactly one JSON object per
// line on stdout, in the same order. Never exits on a failing case.
//
// The comparable artefact is the generated MIME message. It is obtained from
// nodemailer's own stream transport with `buffer: true`, so nothing is ever
// sent over the network.

const fs = require('fs');
const path = require('path');
const readline = require('readline');
const nodemailer = require('nodemailer');

const ROOT = process.env.PARITY_ROOT || path.join(__dirname, '..');

// Pinned so the output is byte-stable: nodemailer would otherwise stamp the
// current time and a random Message-ID.
const FIXED_DATE = new Date(Date.UTC(2026, 0, 2, 15, 4, 5));
const FIXED_MESSAGE_ID = '<parity-fixed@example.com>';

const transport = nodemailer.createTransport({
    streamTransport: true,
    buffer: true,
    newline: 'windows'
});

function resolveFixture(p) {
    if (/^[a-z]+:\/\//i.test(p)) {
        throw new Error('remote attachment paths are not allowed in the parity harness');
    }
    return path.resolve(ROOT, p);
}

// Translate a language-neutral attachment spec into nodemailer's shape.
function buildAttachment(spec) {
    const out = {};
    if (spec.filename !== undefined) out.filename = spec.filename;
    if (spec.contentType !== undefined) out.contentType = spec.contentType;
    if (spec.cid !== undefined) out.cid = spec.cid;
    if (spec.contentDisposition !== undefined) out.contentDisposition = spec.contentDisposition;
    if (spec.contentTransferEncoding !== undefined) out.contentTransferEncoding = spec.contentTransferEncoding;

    const mode = spec.mode || 'string';
    switch (mode) {
        case 'string':
            out.content = spec.content || '';
            break;
        case 'buffer':
            // `content` is base64 in the case file; feed a real Buffer.
            out.content = Buffer.from(spec.content || '', 'base64');
            break;
        case 'base64':
            out.content = spec.content || '';
            out.encoding = 'base64';
            break;
        case 'path':
            out.path = resolveFixture(spec.path);
            break;
        case 'stream':
            out.content = fs.createReadStream(resolveFixture(spec.path));
            break;
        default:
            throw new Error('unknown attachment mode: ' + mode);
    }
    return out;
}

function buildMailOptions(opts) {
    const mail = {
        // Determinism: never let nodemailer generate these.
        date: opts.date ? new Date(opts.date) : FIXED_DATE,
        messageId: opts.messageId !== undefined ? opts.messageId : FIXED_MESSAGE_ID,
        // The Go port emits no X-Mailer header; suppressing it here keeps the
        // comparison about MIME structure rather than about a banner.
        xMailer: false,
        disableUrlAccess: true
    };

    for (const key of ['from', 'sender', 'subject', 'text', 'html', 'watchHtml', 'amp', 'inReplyTo', 'priority', 'textEncoding', 'encoding']) {
        if (opts[key] !== undefined) mail[key] = opts[key];
    }
    for (const key of ['to', 'cc', 'bcc', 'replyTo', 'references']) {
        if (opts[key] !== undefined) mail[key] = opts[key];
    }
    if (opts.headers !== undefined) {
        // Array of {key,value} preserves order and duplicates.
        mail.headers = opts.headers;
    }
    if (opts.list !== undefined) {
        // `headers` and `unsubscribePost` are Go-side-only carriers: nodemailer
        // turns every key of `list` into a List-<key> header, so they must not
        // reach it.
        const list = {};
        for (const [k, v] of Object.entries(opts.list)) {
            if (k === 'headers' || k === 'unsubscribePost') continue;
            list[k] = v;
        }
        mail.list = list;
    }
    if (opts.icalEvent !== undefined) mail.icalEvent = opts.icalEvent;
    if (opts.alternatives !== undefined) {
        mail.alternatives = opts.alternatives.map(a => {
            const alt = { contentType: a.contentType };
            if (a.contentTransferEncoding !== undefined) alt.contentTransferEncoding = a.contentTransferEncoding;
            alt.content = a.content;
            return alt;
        });
    }
    if (opts.attachments !== undefined) {
        mail.attachments = opts.attachments.map(buildAttachment);
    }
    return mail;
}

async function runCase(req) {
    if (req.fn !== 'buildMessage') {
        throw new Error('unknown fn: ' + req.fn);
    }
    const mail = buildMailOptions((req.args && req.args[0]) || {});
    const info = await transport.sendMail(mail);
    return info.message.toString('utf8');
}

const rl = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });

// Serialise the cases: each reply must be written in request order.
let chain = Promise.resolve();

rl.on('line', line => {
    const trimmed = line.trim();
    if (!trimmed) return;
    chain = chain.then(async () => {
        let req;
        try {
            req = JSON.parse(trimmed);
        } catch (err) {
            process.stdout.write(JSON.stringify({ id: '<unparseable>', ok: false, error: 'bad request line: ' + err.message }) + '\n');
            return;
        }
        try {
            const value = await runCase(req);
            process.stdout.write(JSON.stringify({ id: req.id, ok: true, value }) + '\n');
        } catch (err) {
            process.stdout.write(JSON.stringify({ id: req.id, ok: false, error: String((err && err.message) || err) }) + '\n');
        }
    });
});

rl.on('close', () => {
    chain.then(() => process.exit(0));
});
