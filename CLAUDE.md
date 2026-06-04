<!-- nx configuration start-->
<!-- Leave the start & end comments to automatically receive updates. -->

# General Guidelines for working with Nx

- For navigating/exploring the workspace, invoke the `nx-workspace` skill first - it has patterns for querying projects, targets, and dependencies
- When running tasks (for example build, lint, test, e2e, etc.), always prefer running the task through `nx` (i.e. `nx run`, `nx run-many`, `nx affected`) instead of using the underlying tooling directly
- Prefix nx commands with the workspace's package manager (e.g., `pnpm nx build`, `npm exec nx test`) - avoids using globally installed CLI
- You have access to the Nx MCP server and its tools, use them to help the user
- For Nx plugin best practices, check `node_modules/@nx/<plugin>/PLUGIN.md`. Not all plugins have this file - proceed without it if unavailable.
- NEVER guess CLI flags - always check nx_docs or `--help` first when unsure

## Scaffolding & Generators

- For scaffolding tasks (creating apps, libs, project structure, setup), ALWAYS invoke the `nx-generate` skill FIRST before exploring or calling MCP tools

## When to use nx_docs

- USE for: advanced config options, unfamiliar flags, migration guides, plugin configuration, edge cases
- DON'T USE for: basic generator syntax (`nx g @nx/react:app`), standard commands, things you already know
- The `nx-generate` skill handles generator discovery internally - don't call nx_docs just to look up generator syntax


<!-- nx configuration end-->

# Running the API dev server (for agents)

The `api:serve` target builds and `exec`s a single binary (`dist/api`) with graceful shutdown —
a SIGTERM tears down the HTTP server **and** the headless Chrome it spawns. Chrome runs via
flatpak, so broad kills are dangerous (`pkill chrome` can kill the user's real browser).

To run and stop the server during validation:

1. **Start** in the background and capture the `task_id`:
   `Bash(run_in_background: true)` → `pnpm nx run api:serve`
2. **Validate**: `curl -fs localhost:8080/api/hello` returns `Hello, World!`. To exercise Chrome
   rendering use `/api/dashboard/preview?width=800&height=480&colors=0,1,2,3&mock=1` (PNG) or the
   packed image `/api/dashboard/image?width=800&height=480&colors=0,1,2,3&colorDepth=4&mock=1`
   (`colorDepth` is required). The task output file shows `Starting API server on :8080` once ready.
3. **Stop**: `TaskStop({task_id})` — the single exec'd binary shuts down gracefully and kills Chrome.
4. **Verify stopped** (read-only): `ss -ltnp 'sport = :8080'` returns nothing.

**Hard rule:** NEVER use `pkill`, `killall`, `fuser -k`, or kill by port/name. If a `TaskStop` ever
fails, kill only the **specific PID/PGID captured from that task** — never by pattern.

For an interactive auto-restart edit loop (rebuilds on save), use `pnpm nx run api:dev` (wgo) and
stop it with Ctrl-C.