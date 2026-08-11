# Security Policy

## Supported versions

The latest released minor version receives security fixes. Older versions are
best-effort.

## Reporting a vulnerability

**Please do not open a public issue for security problems.**

Use GitHub's private vulnerability reporting instead:

1. Go to the **Security** tab of this repository.
2. Click **Report a vulnerability** (Private vulnerability reporting).

If that is unavailable, email the maintainer at the address on their GitHub
profile. Please include:

- a description of the issue and its impact,
- steps to reproduce (a minimal proof-of-concept if possible),
- affected version(s).

We aim to acknowledge reports within a few days and to ship a fix or mitigation
as promptly as the severity warrants. Once a fix is available we will publish a
GitHub Security Advisory crediting the reporter (unless anonymity is requested).

## Scope notes for this package

This package generates HTML from application data, so a few areas are the most
likely place for a real vulnerability. Reports touching them are especially
welcome:

- **Escaping.** Text children and attribute values are HTML-escaped on the way
  out. An input that escapes its context — breaking out of an attribute, a tag
  name, or a text node — is a genuine XSS vulnerability, not a rendering bug.
- **Attribute and tag names** come from application code, not from user input,
  and are not sanitized. Building a tag name or a props key from untrusted input
  is unsafe by design; do not do it.
- **Unbounded input.** A deeply nested or enormous tree built from untrusted
  input consumes stack and memory during the recursive render. Bound the input
  before rendering it.
- **Denial of service.** Rendering is serialized process-wide, so a component
  that blocks during render blocks every root in the process. Do not perform
  blocking I/O in a component body — that is what `UseEffect` and the `Async`
  helpers are for.

## Automated scanning

This repository is continuously scanned by:

- **CodeQL** code scanning (static analysis),
- **govulncheck** for known vulnerabilities in Go dependencies,
- **Trivy** filesystem scanning (vulnerabilities, secrets, misconfiguration),
- **Gitleaks** secret scanning, and
- GitHub secret scanning with push protection.
