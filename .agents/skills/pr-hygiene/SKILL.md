---
name: pr-hygiene
description: Use when preparing commits, updating PRs, reviewing diffs, or coordinating Claude/Codex/Hermes work in hamburger_evaluation.
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [git, pr, review, hygiene]
    related_skills: [backend-rails-boundaries, frontend-spa-boundaries]
---

# PR Hygiene

## Overview

Use this skill before staging, committing, pushing, or updating PRs. The repo
may contain unrelated local files; keep agent changes explicit and scoped.

## When to Use

- Before `git add`, `git commit`, or `git push`
- Before editing a PR body or closing a PR
- During diff review
- When coordinating Claude Code, Codex, or Hermes Agent work

## Workflow

```bash
git status --short --branch --untracked-files=all
git diff --check
git diff --stat
```

Stage explicit paths only. Do not include unrelated local files such as
`SETUP.md` or `plans/*.md` unless the user requested them.

## Secrets

Never read, stage, summarize, or commit:

- `.env*`
- `backend/.env*`
- `frontend/.env*`
- `secrets/**`
- `backend/.kamal/secrets`
- `backend/config/master.key`

## PR Summary Shape

- Summary: concise bullets of user-visible or architectural changes
- Tests: exact commands run and pass/fail
- Notes: migrations, skipped checks, or follow-up risks

## Common Pitfalls

1. Staging unrelated modified files.
2. Reporting targeted tests as sufficient when full suite is required.
3. Closing or editing PRs without user approval.
4. Leaving generated agent files unverified.

## Verification Checklist

- [ ] `git status --short --branch` reviewed.
- [ ] `git diff --check` passes.
- [ ] Only intended paths are staged/committed.
- [ ] PR body includes commands actually run.
