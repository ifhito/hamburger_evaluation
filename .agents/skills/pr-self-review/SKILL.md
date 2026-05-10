---
name: pr-self-review
description: When preparing a PR, run a scoped self-review before asking humans.
allowed-tools: [Read, Grep, Glob, Bash(git status:*), Bash(git diff:*)]
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [pr, review, git]
    related_skills: [backend-change-validation, frontend-change-validation]
---

# PR Self Review

## Overview

Use this skill to review the current working tree before a PR update or human
review request. The job is to find unrelated changes, risky diffs, and missing
validation evidence.

## When to Use

- Before opening or updating a PR.
- Before asking the user to review generated changes.
- Before staging or committing agent-generated work.

## Job

1. Read `references/pr-self-review.md`.
2. Run the following script:

```bash
.agents/skills/pr-self-review/scripts/pr-self-review.sh
```

3. Summarize only:
   - changed files grouped by intent,
   - possible unrelated files,
   - missing checks,
   - top review risks.

## Output

Return at most five bullets, each with severity and filepath when applicable.

## Common Pitfalls

1. Do not stage files as part of this skill.
2. Do not edit PR text as part of this skill.
3. Do not read secrets or env files.

## Verification Checklist

- [ ] Git status and diff check were inspected.
- [ ] Unrelated files were called out.
- [ ] Required checks were listed from actual evidence.
