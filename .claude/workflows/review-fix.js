export const meta = {
  name: 'review-fix',
  description: 'Review uncommitted changes, auto-fix Critical/Warning findings, re-review until clean (max 3 rounds)',
  whenToUse: 'After implementing changes, when the user wants review findings fixed automatically before commit.',
  phases: [
    { title: 'Review', detail: 'reviewer agent returns structured findings' },
    { title: 'Fix', detail: 'implementer agent applies fixes for actionable findings' },
  ],
}

const FINDINGS_SCHEMA = {
  type: 'object',
  required: ['clean', 'findings'],
  properties: {
    clean: { type: 'boolean', description: 'true when there are no Critical or Warning findings' },
    findings: {
      type: 'array',
      items: {
        type: 'object',
        required: ['severity', 'file', 'reason', 'fix'],
        properties: {
          severity: { type: 'string', enum: ['Critical', 'Warning', 'Suggestion'] },
          file: { type: 'string', description: 'filepath, with :line when known' },
          reason: { type: 'string' },
          fix: { type: 'string', description: 'concrete change to make' },
        },
      },
    },
  },
}

const MAX_ROUNDS = 3
const focus = typeof args === 'string' && args.trim() ? `\nExtra focus requested by the user: ${args.trim()}` : ''
const history = []
let suggestions = []

for (let round = 1; round <= MAX_ROUNDS; round++) {
  const review = await agent(
    `Review ALL uncommitted changes in this repository (staged, unstaged, and untracked source files — inspect via git status / git diff / git diff --cached and Read).
Apply your review priorities (correctness, backend DDD boundary violations, frontend boundary violations, security, missing tests, out-of-scope files).
Only report real issues; do not pad. Style nits that RuboCop/ESLint would catch are out of scope.${focus}
Return findings via the structured output schema. Set clean=true when there are no Critical or Warning findings (Suggestions alone still count as clean).`,
    { agentType: 'reviewer', label: `review:round${round}`, phase: 'Review', schema: FINDINGS_SCHEMA },
  )
  if (!review) return { status: 'aborted', round, history }

  suggestions = review.findings.filter(f => f.severity === 'Suggestion')
  const actionable = review.findings.filter(f => f.severity !== 'Suggestion')
  log(`round ${round}: ${actionable.length} actionable finding(s), ${suggestions.length} suggestion(s)`)

  if (actionable.length === 0) {
    history.push({ round, actionable: 0 })
    return { status: 'clean', rounds: round, history, suggestions }
  }

  const fixReport = await agent(
    `Fix the following review findings in the working tree. Address every finding; if you disagree with one, explain why instead of silently skipping it.

${JSON.stringify(actionable, null, 2)}

Keep each fix minimal and scoped to the finding. Follow the repo boundary rules from your instructions.
Run only the cheapest targeted validation relevant to what you changed (e.g. a single spec file or pnpm run type-check) — the full suite runs later via the stop sensors.
Report per finding: fixed / skipped (with reason), files touched, and validation commands run with pass/fail.`,
    { agentType: 'implementer', label: `fix:round${round}`, phase: 'Fix' },
  )
  history.push({ round, actionable: actionable.length, fixReport })
}

return { status: 'max-rounds-reached', rounds: MAX_ROUNDS, history, suggestions }
