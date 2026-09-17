---
name: github-story
description: Create a GitHub issue as a user story with PRD-lite structure — requirements, spec, acceptance criteria, and definition of done. Use when the user wants to file a story, plan a feature as an issue, or turn a discussion into a ticket.
allowed-tools: [Read, Grep, Glob, Bash(git log:*), Bash(gh issue:*), Bash(gh label:*)]
version: 1.0.0
author: Hamburger Evaluation Agents
license: MIT
metadata:
  hermes:
    tags: [github, issue, story, prd, planning]
    related_skills: [pr-hygiene, backend-go-boundaries, frontend-spa-boundaries]
---

# GitHub Story

## Overview

Turn a feature idea into a GitHub issue that works as a contract: the
orchestrator's Frame step consumes the acceptance criteria directly, and the
definition of done maps onto this repo's pipeline. A story issue is the
upstream artifact of the whole harness.

## Procedure

1. **Elicit** — before writing, resolve with the user (ask only what changes
   the story):
   - Who is the user of this change, and what can they do after it?
   - What is explicitly OUT of scope?
   - Error/edge behavior: what happens when it fails?
   - Any API/schema impact? (drives spec section and sizing)
2. **Draft** — fill the template below. Show the draft to the user before
   creating anything.
3. **Split check** — if the story needs more than roughly 3 implementer
   tasks or touches backend and frontend with independent value, propose
   splitting into multiple stories linked from a parent.
4. **Create** — `gh issue create --title "<title>" --body-file <draft>` with
   labels: `story` plus area labels (`backend-go`, `frontend`). Report the
   issue URL.

## Story Template

```markdown
## 背景 / 課題          ← why this matters, in the user's world
## ゴール               ← outcomes, not implementation
## 非ゴール             ← what this story deliberately does NOT do
## 要件                 ← functional requirements, numbered (R1, R2, …)
## 仕様                 ← concrete contract: endpoints + request/response
                          shapes (snake_case), data model changes, UI
                          behavior, authz rules. Unknowns go to 未解決の問い
## 受け入れ条件          ← numbered (AC1, AC2, …), each testable,
                          Given/When/Then form, MUST include error cases
                          (unauthorized, not found, validation)
## 完了の定義 (DoD)      ← checklist, see below
## 未解決の問い          ← open questions blocking or deferrable, owner per item
## 依存 / リスク         ← other stories, migrations, external services
```

## Acceptance Criteria Rules

- Each criterion is observable from outside (API response, UI state) — never
  "code is clean" or "implemented correctly".
- Error paths are first-class: a story with only happy-path criteria is
  incomplete.
- If a criterion can't be phrased as a test, it belongs in 未解決の問い, not
  in 受け入れ条件.

## Standard DoD (adapt, don't skip)

- [ ] 受け入れ条件それぞれに対応するテストが存在し green
- [ ] backend-go: go-checks.sh 通過 / frontend: type-check + lint + test 通過
- [ ] レビューパイプライン完了(V1/V2 通過、P1 判断ゼロ)
- [ ] Draft PR → ready → merge 済み(PR に `Closes #<issue>`)
- [ ] ドキュメント更新(API 一覧・CLAUDE.md 等、該当時)

## Handoff

When implementation starts, the orchestrator reads the issue
(`gh issue view <n>`), uses 受け入れ条件 as acceptance criteria in its task
specs, and puts `Closes #<n>` in the draft PR body. 未解決の問い must be
empty or explicitly deferred before implementation begins.
