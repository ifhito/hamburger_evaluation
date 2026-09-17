# Hermes Launch Commands

## Main coding session

```bash
hermes chat   --toolsets file,terminal,skills,todo,delegation   --skills backend-go-boundaries,frontend-spa-boundaries,pr-hygiene
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
hermes -w chat   --toolsets file,terminal,skills,todo,delegation   --skills backend-go-boundaries,frontend-spa-boundaries,pr-hygiene
```

## Orchestrator session (implementation + review, delegated)

```bash
hermes chat -q "$(cat .agents/hermes/orchestrator.md)"   --toolsets file,terminal,skills,todo,delegation   --skills backend-go-boundaries,frontend-spa-boundaries,focused-review,pr-hygiene
```

## Implementer session

```bash
hermes chat -q "$(cat .agents/hermes/implementer.md)"   --toolsets file,terminal,skills,todo   --skills backend-go-boundaries,frontend-spa-boundaries
```

## One-shot reviewer

```bash
hermes chat -q "$(cat .agents/hermes/reviewer.md)"   --toolsets file,terminal,skills   --skills pr-hygiene
```

## Shared sensors

```bash
python3 .claude/hooks/stop-sensors.py
```
