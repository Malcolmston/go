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

## Automated scanning

This repository is continuously scanned by:

- **CodeQL** code scanning (static analysis),
- **govulncheck** for known vulnerabilities in Go dependencies,
- **Trivy** filesystem scanning (vulnerabilities, secrets, misconfiguration),
- **Gitleaks** secret scanning, and
- GitHub secret scanning with push protection.
