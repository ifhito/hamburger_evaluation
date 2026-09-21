export const meta = {
  name: 'review-fix',
  description: 'Review uncommitted changes (reviewer + the code-review skill), verify findings (V1), auto-fix what needs no user decision, and return the rest prioritized (V2). Max 3 rounds.',
  whenToUse: 'After implementing changes, when the user wants review findings verified and fixed automatically before commit.',
  phases: [
    { title: 'Review', detail: 'reviewer agent + code-review skill (round 1) return structured findings' },
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

// `/code-review`(Skill の code-review)を、Review の段階の追加の 1 パスにする(1 ラウンド目だけ。重いので)。
// reviewer は Skill ツールを持たないので、Skill を呼べる既定のエージェントに任せる。
// その結果は「未検証」と明記されるので、reviewer の指摘と統合したうえで、必ず V1(verifier)を通す。
const CODE_REVIEW_SCHEMA = {
  type: 'object',
  required: ['ran', 'findings'],
  properties: {
    ran: { type: 'boolean', description: 'true only if the code-review skill really ran and returned its result' },
    note: { type: 'string', description: 'when ran=false: the exact reason (error text)' },
    findings: FINDINGS_SCHEMA.properties.findings,
  },
}
// 重要: code-review は、呼び出したセッションの作業ディレクトリ(主ディレクトリ)の差分を見る。git worktree で作業しているときは、
// 対象のパスを args に明示しないと、別の木(主ディレクトリの未コミットの変更)を黙ってレビューする(実測済み)。
const worktree = (typeof args === 'string' && args.match(/worktree:\s*(\S+)/)?.[1]) || ''
const CODE_REVIEW_TARGET = worktree
  ? `The directory to review is ${worktree} (use \`git -C ${worktree} diff origin/main\`).`
  : `The directory to review is the repository root of your working directory (\`git rev-parse --show-toplevel\`).`
const CODE_REVIEW_PROMPT = `Run the Claude Code \`code-review\` skill on this branch's changes against main. ${CODE_REVIEW_TARGET}
Call the Skill tool with skill "code-review" and put the ABSOLUTE PATH of that directory and the exact diff command in args, and say that changes in any other directory (such as the main checkout) are out of scope — the skill reviews the session's own working directory unless told otherwise. Do NOT review the code yourself instead of the skill.
The skill may run as a background fork: keep your turn open and wait for its result (poll with \`sleep 60\`, up to about 15 minutes) rather than ending the turn.
After it returns, VERIFY the target: every file named in its findings must be in \`git -C <that directory> diff --name-only origin/main\`. If the skill reviewed another tree (for example the main checkout's uncommitted changes), return ran=false with note "reviewed the wrong tree" and findings=[].
If the skill cannot be invoked, fails, or returns no result, return ran=false with the exact error in note and findings=[] — never pretend it ran.
Convert each finding of the skill's output (it may be headed 高/中/低 or be JSON with file/line/summary/failure_scenario) to the schema: severity 高→Critical (security or correctness), 中→Warning, 低→Suggestion; file = path:line; reason = the finding text with its attack/failure scenario; fix = the change it proposes. The skill says its findings are unverified: pass them on as they are (a verifier checks each one afterwards).`

const SEVERITY_ORDER = ['Critical', 'Warning', 'Suggestion']
// 同じ path:line への指摘は 1 つにまとめる(重い方の severity を残し、理由を並べる)。行のない指摘はまとめない。
function mergeFindings(list) {
  const byKey = new Map()
  list.forEach((f, i) => {
    const key = /:\d+/.test(f.file) ? f.file : `${f.file}#${i}`
    const prev = byKey.get(key)
    if (!prev) return byKey.set(key, f)
    const worse = SEVERITY_ORDER.indexOf(f.severity) < SEVERITY_ORDER.indexOf(prev.severity) ? f.severity : prev.severity
    byKey.set(key, { ...prev, severity: worse, reason: `${prev.reason}\n[${f.source}] ${f.reason}`, fix: `${prev.fix}\n[${f.source}] ${f.fix}`, source: `${prev.source}+${f.source}` })
  })
  return [...byKey.values()]
}

const MAX_ROUNDS = 3
const focus = typeof args === 'string' && args.trim() ? `\nExtra focus requested by the user: ${args.trim()}` : ''
const history = []
const userDecisions = []
const discarded = []
const notices = []
let suggestions = []
let codeReview = { ran: false, reason: 'まだ実行していない' }
const result = (status, extra) => ({ status, ...extra, history, userDecisions, discarded, suggestions, codeReview, notices })

for (let round = 1; round <= MAX_ROUNDS; round++) {
  const [review, cr] = await parallel([
    () => agent(
      `Review ALL uncommitted changes in this repository (staged, unstaged, and untracked source files — inspect via git status / git diff / git diff --cached and Read).
Apply your review priorities. Only report real issues; do not pad.${focus}
Return findings via the structured output schema. Set clean=true when there are no Critical or Warning findings.`,
      { agentType: 'reviewer', label: `review:round${round}`, phase: 'Review', schema: FINDINGS_SCHEMA },
    ),
    () => round === 1
      ? agent(CODE_REVIEW_PROMPT + focus, { label: 'code-review', phase: 'Review', schema: CODE_REVIEW_SCHEMA })
      : Promise.resolve(null),
  ])
  if (!review) return result('aborted', { round })

  if (round === 1) {
    codeReview = cr?.ran ? { ran: true } : { ran: false, reason: cr?.note || 'エージェントが失敗した、または結果を返さなかった' }
    // 黙って飛ばさない: 未実行なら、その理由と、手動で実行する依頼を、結果に必ず載せる
    if (!codeReview.ran) notices.push(`code-review 未実行(${codeReview.reason})。手動で /code-review を実行し、結果の要約を PR のコメントに残してください。`)
  }
  const merged = mergeFindings([
    ...review.findings.map(f => ({ ...f, source: 'reviewer' })),
    ...(cr?.ran ? cr.findings : []).map(f => ({ ...f, source: 'code-review' })),
  ])

  suggestions = merged.filter(f => f.severity === 'Suggestion')
  const actionable = merged.filter(f => f.severity !== 'Suggestion')
  if (actionable.length === 0) {
    history.push({ round, actionable: 0 })
    return result('clean', { rounds: round })
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
    return result('no-auto-fixable', { rounds: round })
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
    return result('converged', { rounds: round })
  }
}

return result('max-rounds-reached', { rounds: MAX_ROUNDS })
