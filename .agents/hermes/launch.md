# Hermes Launch Commands

## Main coding session

```bash
hermes chat   --toolsets file,terminal,skills,todo,delegation   --skills backend-rails-boundaries,frontend-spa-boundaries,pr-hygiene
```

## Read-only investigation

```bash
hermes chat   --toolsets file,terminal,skills   --skills pr-hygiene
```

## Reviewer session

```bash
hermes chat   --toolsets file,terminal,skills   --skills pr-hygiene
```

## Worktree mode for isolated edits

```bash
hermes -w chat   --toolsets file,terminal,skills,todo,delegation   --skills backend-rails-boundaries,frontend-spa-boundaries,pr-hygiene
```

## One-shot reviewer

```bash
hermes chat -q "$(cat .agents/hermes/reviewer.md)"   --toolsets file,terminal,skills   --skills pr-hygiene
```

## Shared sensors

```bash
python3 .claude/hooks/stop-sensors.py
```
