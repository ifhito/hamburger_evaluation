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
  `args: "authorization checks in the reviews endpoints"`. To focus on one
  lens from the [[focused-review]] skill, name it, e.g.
  `args: "apply the focused-review consistency lens"`.

Do not re-implement the loop manually with the Agent tool; the workflow is the
single source of truth for round limits and finding schema.

## After the Workflow Returns

Report in Japanese. The user sees ONLY what needs their judgment:

1. **Present `userDecisions` first**, ordered P1 → P2 → P3, each as one
   decision question with options, a recommendation, and impact. At most 5
   up front; overflow in an appendix. P1 items block the work.
2. **Auto-fixed findings**: one summary line per round (count + what kind).
   Do not ask the user to re-approve them.
3. **`discarded` (false positives)**: one line noting the count; evidence
   stays available on request. Never present them as questions.
4. `status: "no-auto-fixable"` means everything left needs the user — say so
   plainly. `status: "max-rounds-reached"` — list still-open items and stop;
   do not keep looping on your own.
5. Remind that full validation still runs via the stop sensors
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
