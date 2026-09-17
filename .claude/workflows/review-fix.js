export const meta = {
  name: 'review-fix',
  description: 'Review uncommitted changes, verify findings (V1), auto-fix what needs no user decision, and return the rest prioritized (V2). Max 3 rounds.',
  whenToUse: 'After implementing changes, when the user wants review findings verified and fixed automatically before commit.',
  phases: [
    { title: 'Review', detail: 'reviewer agent returns structured findings' },
    { title: 'Verify', detail: 'verifier agent confirms/refutes each finding (V1)' },
    { title: 'Fix', detail: 'implementer applies auto-fix-safe findings' },
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

const VERDICT_SCHEMA = {
  type: 'object',
  required: ['verdict', 'evidence', 'autoFixSafe'],
  properties: {
    verdict: { type: 'string', enum: ['CONFIRMED', 'FALSE_POSITIVE', 'UNCERTAIN'] },
    evidence: { type: 'string', description: 'filepath:line evidence for the verdict' },
    severity: { type: 'string', enum: ['Critical', 'Warning', 'Suggestion'], description: 'corrected severity' },
    autoFixSafe: {
      type: 'boolean',
      description: 'true only if CONFIRMED and the fix is local: no API shape, schema, endpoint-semantics, dependency, or user-visible behavior changes, and consistent with repo boundary skills',
    },
    decisionQuestion: { type: 'string', description: 'when not autoFixSafe: the single question the user must answer' },
    priority: { type: 'string', enum: ['P1', 'P2', 'P3'], description: 'P1 blocks, P2 decide now, P3 optional' },
  },
}

const MAX_ROUNDS = 3
const focus = typeof args === 'string' && args.trim() ? `\nExtra focus requested by the user: ${args.trim()}` : ''
const history = []
const userDecisions = []
const discarded = []
let suggestions = []

for (let round = 1; round <= MAX_ROUNDS; round++) {
  const review = await agent(
    `Review ALL uncommitted changes in this repository (staged, unstaged, and untracked source files — inspect via git status / git diff / git diff --cached and Read).
Apply your review priorities. Only report real issues; do not pad.${focus}
Return findings via the structured output schema. Set clean=true when there are no Critical or Warning findings.`,
    { agentType: 'reviewer', label: `review:round${round}`, phase: 'Review', schema: FINDINGS_SCHEMA },
  )
  if (!review) return { status: 'aborted', round, history, userDecisions, discarded }

  suggestions = review.findings.filter(f => f.severity === 'Suggestion')
  const actionable = review.findings.filter(f => f.severity !== 'Suggestion')
  if (actionable.length === 0) {
    history.push({ round, actionable: 0 })
    return { status: 'clean', rounds: round, history, userDecisions, discarded, suggestions }
  }

  // V1: verify every actionable finding in parallel
  const verified = await parallel(actionable.map(f => () =>
    agent(
      `Adversarially verify this single review finding against the actual code. Read the real code, trace guards and call paths, and try to construct the concrete failure. Judge autoFixSafe per your instructions.

Finding: ${JSON.stringify(f)}`,
      { agentType: 'verifier', label: `verify:${f.file}`, phase: 'Verify', schema: VERDICT_SCHEMA },
    ).then(v => ({ finding: f, verdict: v }))
  ))

  // V2: triage
  const autoFix = []
  for (const { finding, verdict } of verified) {
    if (!verdict) continue
    if (verdict.verdict === 'FALSE_POSITIVE') {
      discarded.push({ ...finding, evidence: verdict.evidence })
    } else if (verdict.verdict === 'CONFIRMED' && verdict.autoFixSafe) {
      autoFix.push({ ...finding, severity: verdict.severity ?? finding.severity, evidence: verdict.evidence })
    } else {
      userDecisions.push({
        ...finding,
        verdict: verdict.verdict,
        evidence: verdict.evidence,
        decisionQuestion: verdict.decisionQuestion ?? `Fix this? ${finding.reason}`,
        priority: verdict.priority ?? 'P2',
      })
    }
  }
  log(`round ${round}: ${actionable.length} findings -> ${autoFix.length} auto-fix, ${userDecisions.length} user decisions, ${discarded.length} discarded`)
  history.push({ round, actionable: actionable.length, autoFix: autoFix.length })

  if (autoFix.length === 0) {
    return { status: 'no-auto-fixable', rounds: round, history, userDecisions, discarded, suggestions }
  }

  const fixReport = await agent(
    `Fix the following verified review findings in the working tree. Address every finding; if you disagree with one, explain why in your report instead of silently skipping it.

${JSON.stringify(autoFix, null, 2)}

Keep each fix minimal and scoped to the finding — these are pre-verified as contract-safe, so do NOT change API shapes, schemas, or behavior beyond removing the bug.
Run only the cheapest targeted validation relevant to what you changed; the full suite runs later via the stop sensors.
Report per finding: fixed / skipped (with reason), files touched, and validation commands run with pass/fail.`,
    { agentType: 'implementer', label: `fix:round${round}`, phase: 'Fix' },
  )
  history.push({ round, fixReport })

  // 収束条件: このラウンドの自動修正に Critical が無ければ、修正適用をもって終了。
  // 再レビューは「Critical を直した」ラウンドの後だけ回す。
  const hadCritical = autoFix.some(f => f.severity === 'Critical')
  if (!hadCritical) {
    return { status: 'converged', rounds: round, history, userDecisions, discarded, suggestions }
  }
}

return { status: 'max-rounds-reached', rounds: MAX_ROUNDS, history, userDecisions, discarded, suggestions }
