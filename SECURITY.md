# Security Policy

## Supported versions

MOD-ARCH does not publish versioned releases yet. Security fixes are applied to the `main` branch and to the latest deployment of the public demo. Older commits and forks are not supported; if you run your own instance, update to the current `main`.

## Reporting a vulnerability

Please report vulnerabilities privately using GitHub private vulnerability reporting:

https://github.com/jeanmarcos-dev/mod-arch/security/advisories/new

Include a description of the issue, the affected component or endpoint, steps to reproduce, and the impact you observed. A proof of concept against your own instance is ideal.

Response time is best effort, usually within 7 days. You will be kept informed of the progress of the fix, and credited in the advisory if you wish.

## What not to do

- Do not open public issues, pull requests or discussions that describe an unpatched vulnerability.
- Do not test against the public demo (https://mod-arch-simulator.com) beyond normal use of the application. Run your own instance for anything more intrusive.
- Do not run load, stress or denial-of-service tests against the public demo.
- Do not access, modify or exfiltrate data that belongs to other users, and do not attempt to brute-force credentials.

## Scope

In scope: the code in this repository (Go backend, Vue frontend, deployment files).

Out of scope: vulnerabilities in third-party services the application can call (LLM and embeddings providers, SMTP servers, Cloudflare Turnstile), in your reverse proxy or operating system, and findings that require an already compromised host or admin account.

## Deployment hardening

For anyone self-hosting MOD-ARCH:

- **Set `SESSION_SECRET`** to a long random value (`make session-secret` prints one). It signs admin cookies, keys the at-rest encryption of API keys and SMTP passwords stored in the database, and salts the stored hashes of client IP addresses. Keep it stable across restarts and out of version control.
- **Set `ALLOWED_ORIGINS`** to the exact host names that serve the application. Without it, WebSocket upgrades are accepted only from the local development origins.
- **Run behind TLS.** The backend does not terminate TLS. Put it behind a reverse proxy that does, and forward WebSocket upgrades. Do not expose the plain HTTP port to the internet.
- **Configure `TRUSTED_PROXY_HEADER`** with the header your proxy sets to the real client IP, for example `CF-Connecting-IP` behind Cloudflare or `X-Real-IP` behind nginx. This keeps rate limits and per-client accounting tied to the real client address. Only set it when the proxy overwrites that header and the backend is reachable only through the proxy; otherwise clients can spoof it.
- **Keep the admin panel disabled unless you need it.** It is enabled only when `ADMIN_USER`, `ADMIN_PASS_HASH` and `SESSION_SECRET` are all set. Use a strong password, generate the hash with `make admin-hash`, and restrict access to `/admin` at the proxy if you can.
- **Protect the database.** The SQLite file contains user records, session history and, if the tutor is on, student questions. Keep it on a persistent volume with restricted permissions (set `MODOS_STATE_DB` to its path) and back it up.
- **Review the tutor before enabling it.** With the tutor on, student questions, conversation history and a compact view of the simulator state are sent to the LLM and embeddings providers you configure. Use provider API keys with spend limits, and tell your users what is recorded.
- **Keep dependencies updated** (`go.mod` and `web/package.json`) and run the backend as an unprivileged user, as the provided Dockerfile and systemd unit do.
