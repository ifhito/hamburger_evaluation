# Hermes Agent Settings

Use this repository from the repo root.

Recommended toolsets:

```bash
hermes chat --toolsets file,terminal,skills,todo,delegation
```

Read-only investigation:

```bash
hermes chat --toolsets file,terminal,skills
```

Load project skills when useful:

```bash
hermes chat --skills backend-go-boundaries,frontend-spa-boundaries,pr-hygiene
```

Do not read:

- `.env` / `.env.*` (in any directory)
- `secrets/**`

Use `AGENTS.md` as the primary project instruction and `.agents/skills/*` as reusable project workflows.
