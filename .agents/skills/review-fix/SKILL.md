---
name: review-fix
description: Review uncommitted changes with the reviewer agent, auto-fix Critical/Warning findings with the implementer agent, and re-review until clean. Use before committing or when the user asks to "review and fix".
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [review, fix, automation, git]
    related_skills: [pr-hygiene, pr-self-review]
---

# Review Fix Loop

## Overview

Automates the review → fix → re-review cycle over the current working tree
using the repo's `reviewer` and `implementer` agents. The loop stops when a
round produces no Critical/Warning findings, or after 3 rounds.

## How to Run

Invoke the saved workflow (this skill is your authorization to call it):

- `Workflow` tool with `name: "review-fix"`.
- Pass any user-requested focus area as `args` (a plain string), e.g.
  `args: "authorization checks in reviews_controller"`.

Do not re-implement the loop manually with the Agent tool; the workflow is the
single source of truth for round limits and finding schema.

## After the Workflow Returns

1. Report per round: findings found, fixed, skipped (with the implementer's
   reasons). Report in Japanese.
2. `status: "clean"` — summarize remaining Suggestions (not auto-fixed by
   design) and let the user decide on them.
3. `status: "max-rounds-reached"` — list the still-open findings and ask the
   user how to proceed. Do not keep looping on your own.
4. Remind that full validation still runs via the stop sensors
   (`python3 .claude/hooks/stop-sensors.py`) — the loop only runs cheap
   targeted checks.

## Guardrails

- The loop never stages, commits, or pushes; that stays with the main session
  and the `pr-hygiene` skill.
- Suggestions are intentionally not auto-fixed — auto-applying opinions is how
  scope creep starts. Surface them instead.
- If the same finding survives two rounds, treat it as a disagreement between
  reviewer and implementer and escalate to the user rather than burning the
  last round.
