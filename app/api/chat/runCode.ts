// app/api/chat/runCode.ts — the assistant's "run code and see the result" tool.
//
// Provisions an ephemeral Vercel Sandbox (Firecracker microVM), writes a small
// set of model-authored files into it, runs one build/run command, and returns
// { stdout, stderr, exitCode, durationMs, timedOut }. The model uses this to
// verify a snippet before presenting it, or to debug on request.
//
// This module is deliberately kept out of route.ts so the route stays readable;
// route.ts only asks whether execution is enabled and, if so, mounts the tool
// returned by makeRunCodeTool().
//
// SECURITY POSTURE (this is a public, CORS-open, unauthenticated endpoint):
//   - Opt-in only. The tool is mounted only when CHAT_SANDBOX_ENABLED is truthy.
//     When off, route.ts never registers it and tells the model execution is
//     unavailable — the assistant keeps working, it just cannot run code.
//   - No secrets in the sandbox. We never forward the app's env / OIDC token /
//     API keys into the VM: Sandbox.create() gets NO `env`, and runCommand()
//     gets NO `env`. The only things that enter the VM are the model's files.
//     Vercel credentials are used solely as create() auth parameters.
//   - Bounded. Hard wall-clock timeout (create `timeout` + runCommand
//     `timeoutMs` + an AbortController race), captured-output size cap, and file
//     / total-code-size caps enforced in the zod schema.
//   - Rate limited. A per-conversation counter (makeRunCodeTool is called once
//     per request) caps how many runCode calls one turn may make.
//   - Leak-free. The sandbox is always torn down in a `finally`.
//   - stdout/stderr are returned as plain strings and clipped; they are never
//     interpreted as anything but text.

import { tool } from 'ai';
import { z } from 'zod';
import { Sandbox } from '@vercel/sandbox';

// --- Bounds (kept consistent with route.ts MAX_* constants) ----------------
const MAX_FILES = 6; // files the model may write per run
const MAX_FILE_BYTES = 20_000; // per-file source size
const MAX_TOTAL_BYTES = 40_000; // total source across all files
const MAX_PATH_LEN = 128; // per-file relative path length
const MAX_CMD_LEN = 300; // optional model-supplied command
const MAX_OUTPUT_BYTES = 16_000; // captured stdout/stderr each, returned to model
const RUN_TIMEOUT_MS = 30_000; // hard wall-clock per run
const SANDBOX_TIMEOUT_MS = 60_000; // VM auto-terminate ceiling (> run timeout)
export const MAX_RUNS_PER_CONVERSATION = 3; // per-request runCode budget

// The languages the site is actually about: Go (the ports), plus Node/TypeScript
// and Python (the upstreams these libraries are ported from).
const LANGUAGES = ['go', 'node', 'typescript', 'python'] as const;
type Language = (typeof LANGUAGES)[number];

// Library ids are generated slugs (mirrors LIBRARY_RE in route.ts); a Go module
// fetch target must look like one before we build github.com/malcolmston/<id>.
const LIBRARY_RE = /^[a-z0-9][a-z0-9._-]{0,63}$/;

// A relative, non-escaping path: no leading slash, no "..", no NUL.
const SAFE_PATH_RE = /^(?!.*(^|\/)\.\.(\/|$))[A-Za-z0-9._][A-Za-z0-9._\/-]*$/;

export interface RunCodeResult {
  stdout: string;
  stderr: string;
  exitCode: number;
  durationMs: number;
  timedOut: boolean;
  // Present only when the run could not be attempted (misconfig, provisioning
  // failure, unsupported toolchain). The model surfaces this to the user
  // instead of a fabricated result.
  note?: string;
}

// Opt-in gate. Sandbox execution is enabled only when the flag is explicitly
// set AND the SDK has something to authenticate with (Vercel OIDC token on a
// deployment, or an explicit token trio for local/self-hosted). If the flag is
// on but no credentials exist, we still report "enabled" so the tool mounts and
// returns a clear degraded note rather than the route silently dropping it.
export function sandboxExecutionEnabled(): boolean {
  const flag = (process.env.CHAT_SANDBOX_ENABLED ?? '').trim().toLowerCase();
  return flag === '1' || flag === 'true' || flag === 'yes' || flag === 'on';
}

// Resolve Vercel credentials without ever reading app secrets into the VM.
// On Vercel, OIDC is automatic (VERCEL_OIDC_TOKEN) and no trio is needed.
function sandboxCredentials():
  | { token: string; teamId: string; projectId: string }
  | undefined {
  const { VERCEL_TOKEN, VERCEL_TEAM_ID, VERCEL_PROJECT_ID } = process.env;
  if (VERCEL_TOKEN && VERCEL_TEAM_ID && VERCEL_PROJECT_ID) {
    return { token: VERCEL_TOKEN, teamId: VERCEL_TEAM_ID, projectId: VERCEL_PROJECT_ID };
  }
  return undefined; // fall back to automatic OIDC inside the SDK
}

// Clip to a byte budget, marking truncation. Output is always treated as opaque
// text — never parsed, never interpreted as markup or instructions downstream.
function clipBytes(s: string, max: number): string {
  if (s.length <= max) return s;
  return s.slice(0, max) + `\n…[truncated, ${s.length - max} more chars]`;
}

// Per-language toolchain + default command. All commands avoid the network:
//   - TypeScript runs via Node's native type-stripping (Node ≥22.6 / node24),
//     the same mechanism this repo uses in its own scripts — no npm install.
//   - Go uses the universal image and `go run .`; if the image has no Go
//     toolchain we detect exit 127 and degrade clearly rather than hang.
interface Toolchain {
  // Either a managed runtime or an image is passed to Sandbox.create().
  create: { runtime: 'node24' | 'python3.13' } | { image: 'universal' };
  entry: string; // default entry file if the model didn't supply a command
  command: string[]; // default argv
}

function toolchainFor(language: Language): Toolchain {
  switch (language) {
    case 'node':
      return { create: { runtime: 'node24' }, entry: 'index.js', command: ['node', 'index.js'] };
    case 'typescript':
      return {
        create: { runtime: 'node24' },
        entry: 'index.ts',
        command: ['node', '--experimental-strip-types', 'index.ts'],
      };
    case 'python':
      return { create: { runtime: 'python3.13' }, entry: 'main.py', command: ['python3', 'main.py'] };
    case 'go':
      return { create: { image: 'universal' }, entry: 'main.go', command: ['go', 'run', '.'] };
  }
}

const inputSchema = z.object({
  language: z.enum(LANGUAGES).describe('Which toolchain to run: go, node, typescript, or python.'),
  files: z
    .array(
      z.object({
        path: z
          .string()
          .min(1)
          .max(MAX_PATH_LEN)
          .describe('Relative file path, e.g. "main.go" or "index.ts". No leading slash, no "..".'),
        content: z.string().max(MAX_FILE_BYTES).describe('The file contents.'),
      }),
    )
    .min(1)
    .max(MAX_FILES)
    .describe('The program files to write. Keep it small; one main file is the common case.'),
  command: z
    .string()
    .max(MAX_CMD_LEN)
    .optional()
    .describe(
      'Optional shell command to run instead of the default (e.g. "go test ./..."). Runs via `sh -c`. Omit to use the language default (go run . / node index.js / node --experimental-strip-types index.ts / python3 main.py).',
    ),
  goModule: z
    .string()
    .max(64)
    .optional()
    .describe(
      'Go only: a malcolmston library id (e.g. "express", "jwt") to demonstrate. When set, a go.mod is created requiring github.com/malcolmston/<id> and `go mod tidy` runs first. Needs network; degrades clearly if unavailable.',
    ),
});

type RunCodeInput = z.infer<typeof inputSchema>;

// Validate paths + total size beyond what zod checks per-field. Returns a note
// string on rejection, or null when the files are acceptable.
function validateFiles(files: RunCodeInput['files']): string | null {
  let total = 0;
  const seen = new Set<string>();
  for (const f of files) {
    if (!SAFE_PATH_RE.test(f.path)) return `Rejected: unsafe file path "${clipBytes(f.path, 80)}".`;
    if (seen.has(f.path)) return `Rejected: duplicate file path "${f.path}".`;
    seen.add(f.path);
    total += Buffer.byteLength(f.content, 'utf8');
  }
  if (total > MAX_TOTAL_BYTES) {
    return `Rejected: total source ${total} bytes exceeds the ${MAX_TOTAL_BYTES}-byte cap.`;
  }
  return null;
}

// The actual provisioning + run. Sandbox is torn down in `finally`, always.
async function executeInSandbox(input: RunCodeInput): Promise<RunCodeResult> {
  const filesRejection = validateFiles(input.files);
  if (filesRejection) {
    return { stdout: '', stderr: '', exitCode: -1, durationMs: 0, timedOut: false, note: filesRejection };
  }

  const tc = toolchainFor(input.language);
  const creds = sandboxCredentials();
  const started = Date.now();

  // Belt-and-suspenders wall clock: even if the SDK timeouts slip, we abort.
  const ac = new AbortController();
  const killTimer = setTimeout(() => ac.abort(), RUN_TIMEOUT_MS + 5_000);

  let sandbox: Awaited<ReturnType<typeof Sandbox.create>> | undefined;
  try {
    // NOTE: no `env` is passed here — the VM inherits none of the app's secrets.
    sandbox = await Sandbox.create({
      ...(creds ?? {}),
      ...tc.create,
      timeout: SANDBOX_TIMEOUT_MS,
      signal: ac.signal,
    });

    // Build the file set. For a Go module demo, synthesize a go.mod (unless the
    // model already supplied one) so `go mod tidy` / `go run` resolve the port.
    const files = input.files.map((f) => ({ path: f.path, content: f.content }));
    let goModuleNote: string | undefined;
    if (input.language === 'go' && input.goModule) {
      const id = input.goModule.trim().toLowerCase();
      if (!LIBRARY_RE.test(id)) {
        return {
          stdout: '',
          stderr: '',
          exitCode: -1,
          durationMs: Date.now() - started,
          timedOut: false,
          note: `Rejected: "${clipBytes(input.goModule, 40)}" is not a valid malcolmston library id.`,
        };
      }
      if (!files.some((f) => f.path === 'go.mod')) {
        files.push({
          path: 'go.mod',
          content: `module sandbox\n\ngo 1.23\n\nrequire github.com/malcolmston/${id} v0.0.0\n`,
        });
      }
      goModuleNote = `Attempted to fetch github.com/malcolmston/${id}; if the sandbox has no network the fetch fails and this run degrades.`;
    }

    await sandbox.writeFiles(files);

    // Go with a module dependency needs its graph resolved first. This requires
    // network; on failure we still return its stderr so the degrade is visible.
    if (input.language === 'go' && input.goModule) {
      const tidy = await sandbox.runCommand('go', ['mod', 'tidy'], {
        timeoutMs: RUN_TIMEOUT_MS,
        signal: ac.signal,
      });
      if (tidy.exitCode !== 0) {
        const stderr = clipBytes(await tidy.stderr(), MAX_OUTPUT_BYTES);
        return {
          stdout: clipBytes(await tidy.stdout(), MAX_OUTPUT_BYTES),
          stderr,
          exitCode: tidy.exitCode,
          durationMs: Date.now() - started,
          timedOut: false,
          note:
            'go mod tidy failed — the sandbox likely has no network access to fetch the module. ' +
            (goModuleNote ?? ''),
        };
      }
    }

    // Run the command. A custom command goes through `sh -c`; the default argv
    // runs directly. Either way, no env is forwarded.
    const finished = input.command
      ? await sandbox.runCommand('sh', ['-c', input.command], {
          timeoutMs: RUN_TIMEOUT_MS,
          signal: ac.signal,
        })
      : await sandbox.runCommand(tc.command[0], tc.command.slice(1), {
          timeoutMs: RUN_TIMEOUT_MS,
          signal: ac.signal,
        });

    const stdout = clipBytes(await finished.stdout(), MAX_OUTPUT_BYTES);
    const stderr = clipBytes(await finished.stderr(), MAX_OUTPUT_BYTES);
    // A missing toolchain (e.g. no Go in the image) surfaces as exit 127; make
    // that legible rather than presenting it as a program error.
    const note =
      finished.exitCode === 127
        ? `The ${input.language} toolchain was not found in the sandbox image.`
        : goModuleNote;

    return {
      stdout,
      stderr,
      exitCode: finished.exitCode,
      durationMs: Date.now() - started,
      timedOut: false,
      note,
    };
  } catch (err) {
    // An abort (our wall clock) is reported as a timeout, not a hang or a 500.
    const timedOut = ac.signal.aborted || (err instanceof Error && err.name === 'AbortError');
    if (timedOut) {
      return {
        stdout: '',
        stderr: '',
        exitCode: -1,
        durationMs: Date.now() - started,
        timedOut: true,
      };
    }
    // Provisioning / SDK errors (bad or missing credentials, quota) are logged
    // server-side; the model gets a clear, non-sensitive note.
    console.error('[chat] runCode sandbox error', err);
    return {
      stdout: '',
      stderr: '',
      exitCode: -1,
      durationMs: Date.now() - started,
      timedOut: false,
      note: 'Code execution could not be completed (sandbox provisioning failed).',
    };
  } finally {
    clearTimeout(killTimer);
    // Tear the VM down no matter what — never leak a running sandbox.
    if (sandbox) {
      try {
        await sandbox.stop();
      } catch (stopErr) {
        console.error('[chat] runCode sandbox stop failed', stopErr);
      }
    }
  }
}

// Build the tool for one request. The closure holds a per-conversation run
// budget so a single turn cannot spawn unbounded sandboxes.
export function makeRunCodeTool() {
  let runs = 0;
  return tool({
    description:
      'Run a small program you just wrote in an isolated sandbox and see its output, to verify a snippet before presenting it or to debug on request. Languages: go, node, typescript, python. Returns { stdout, stderr, exitCode, durationMs, timedOut }. Keep programs tiny and self-contained.',
    inputSchema,
    execute: async (input) => {
      if (runs >= MAX_RUNS_PER_CONVERSATION) {
        const result: RunCodeResult = {
          stdout: '',
          stderr: '',
          exitCode: -1,
          durationMs: 0,
          timedOut: false,
          note: `Run limit reached (${MAX_RUNS_PER_CONVERSATION} executions per conversation). Explain your reasoning without another run.`,
        };
        return result;
      }
      runs++;
      try {
        return await executeInSandbox(input);
      } catch (err) {
        // execute() must never throw — that would abort the whole stream.
        console.error('[chat] runCode failed', err);
        const result: RunCodeResult = {
          stdout: '',
          stderr: '',
          exitCode: -1,
          durationMs: 0,
          timedOut: false,
          note: 'Code execution failed unexpectedly.',
        };
        return result;
      }
    },
  });
}
