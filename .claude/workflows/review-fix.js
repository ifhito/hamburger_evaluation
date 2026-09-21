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
  required: ['ran', 'reviewedRoot', 'diffFileCount', 'findings'],
  properties: {
    ran: { type: 'boolean', description: 'true only if the code-review skill really ran and returned its result' },
    note: { type: 'string', description: 'when ran=false: the exact reason (error text)' },
    reviewedRoot: { type: 'string', description: 'absolute path of the directory the skill actually reviewed (what it reports as its target)' },
    diffFileCount: { type: 'number', description: 'number of files in that directory\'s diff against the merge base (0 = nothing was reviewed)' },
    findings: FINDINGS_SCHEMA.properties.findings,
  },
}
// 重要: code-review は、呼び出したセッションの作業ディレクトリ(主ディレクトリ)の差分を見る。git worktree で作業しているときは、
// 対象のパスを args に明示しないと、別の木(主ディレクトリの未コミットの変更)を黙ってレビューする(実測済み)。
const worktree = (typeof args === 'string' && args.match(/worktree:\s*(\S+)/)?.[1]) || ''
// reviewer / verifier / implementer も、既定ではセッションの作業ディレクトリ(主ディレクトリ)を見る。worktree のときは、全員にパスを伝える。
const where = worktree
  ? `\nAll work happens in the git worktree ${worktree}: read and edit files by absolute path under it and run git as \`git -C ${worktree} ...\`. Ignore the main checkout and every other directory.`
  : ''
// 差分の基準は、古いローカルの main ではなく、fetch した origin/main と HEAD の merge-base(古い main から切った worktree で、新しいコミットの逆向きの差分が混ざらないように)
const CODE_REVIEW_TARGET = worktree
  ? `The directory to review is ${worktree}. Run \`git -C ${worktree} fetch origin\` first, then the diff is \`git -C ${worktree} diff $(git -C ${worktree} merge-base origin/main HEAD)\` (committed and uncommitted changes; untracked files are covered by the reviewer pass).`
  : `The directory to review is the repository root of your working directory (\`git rev-parse --show-toplevel\`). Run \`git fetch origin\` first; the diff is \`git diff $(git merge-base origin/main HEAD)\`.`
const CODE_REVIEW_PROMPT = `Run the Claude Code \`code-review\` skill on this branch's changes against main. ${CODE_REVIEW_TARGET}
Call the Skill tool with skill "code-review" and put the ABSOLUTE PATH of that directory and the exact diff command in args, and say that changes in any other directory (such as the main checkout) are out of scope — the skill reviews the session's own working directory unless told otherwise. Do NOT review the code yourself instead of the skill. Use the default effort only (never \`ultra\` or any cloud mode: a non-interactive agent cannot answer its prompts), never pass \`--fix\` or \`--comment\`, and do not edit any file or post any comment.
The skill runs as a background fork and takes a long time (about 10-25 minutes for a mid-size diff): keep your turn open and wait for its result rather than ending the turn. The result arrives as a notification between your tool calls, so keep running short commands such as \`date && sleep 25\` (a command that STARTS with a long \`sleep\` is blocked in this environment) for up to about 30 minutes.
Report in reviewedRoot the absolute path of the directory the skill says it reviewed, and in diffFileCount the number of files in that directory's diff. If the skill reviewed another tree (for example the main checkout's uncommitted changes) or the diff is empty, return ran=false with the reason in note and findings=[].
If the skill cannot be invoked, fails, or returns no result, return ran=false with the exact error in note and findings=[] — never pretend it ran.
Convert each finding of the skill's output (it may be headed 高/中/低, or be a JSON array ranked most-severe first with only file/line/summary/failure_scenario) to the schema. severity: 高→Critical (security or correctness), 中→Warning, 低→Suggestion; for the JSON variant use Critical when there is a concrete security or correctness failure scenario, Warning for robustness/operations, Suggestion for pure maintenance/design, and for reuse, simplification, naming and duplication findings (they are opinions and must never be auto-fixed). file = path:line (repo-relative); reason = the finding text with its attack/failure scenario. fix = the change the skill proposes; when it proposes none, write exactly "(no proposed fix: decide after verification)" — never invent one. The skill says its findings are unverified: pass them on as they are (a verifier checks each one afterwards).`

const SEVERITY_ORDER = ['Critical', 'Warning', 'Suggestion']
// 別々のパス(reviewer と code-review)が同じ場所を指したときだけ、1 つにまとめる(重い方の severity を残し、理由を並べる)。
// 同じパスの中の複数の指摘(同じ行の別々のバグ)は、まとめない。Suggestion と、行のない指摘も、まとめない。
const norm = (file) => file.replace(worktree ? `${worktree.replace(/\/$/, '')}/` : '', '').replace(/(:\d+)-\d+$/, '$1')
function mergeFindings(list) {
  const out = []
  for (const f of list) {
    const key = norm(f.file)
    const mergeable = /:\d+/.test(key) && f.severity !== 'Suggestion'
    const prev = mergeable && out.find(g => g.sources.length === 1 && g.key === key && g.severity !== 'Suggestion' && !g.sources.includes(f.source))
    if (!prev) { out.push({ ...f, key, sources: [f.source] }); continue }
    if (SEVERITY_ORDER.indexOf(f.severity) < SEVERITY_ORDER.indexOf(prev.severity)) prev.severity = f.severity
    prev.reason = `${prev.reason}\n[${f.source}] ${f.reason}`
    prev.fix = `${prev.fix}\n[${f.source}] ${f.fix}`
    prev.sources.push(f.source)
    prev.source = prev.sources.join('+')
  }
  return out.map(({ key, sources, ...f }) => f)
}

const MAX_ROUNDS = 3
const focusText = typeof args === 'string' ? args.replace(/worktree:\s*\S+/, '').trim() : ''
const focus = focusText ? `\nExtra focus requested by the user: ${focusText}` : ''
const history = []
const userDecisions = []
const discarded = []
const notices = []
let suggestions = []
let codeReview = { ran: false, reason: 'まだ実行していない' }
// complete=false: 必須の code-review が抜けている。status が 'clean' などでも、完了とは扱わない。
const result = (status, extra) => ({ status, ...extra, complete: codeReview.ran, history, userDecisions, discarded, suggestions, codeReview, notices })

for (let round = 1; round <= MAX_ROUNDS; round++) {
  const [review, cr] = await parallel([
    () => agent(
      `Review ALL uncommitted changes in this repository (staged, unstaged, and untracked source files — inspect via git status / git diff / git diff --cached and Read).
Apply your review priorities. Only report real issues; do not pad.${focus}
Return findings via the structured output schema. Set clean=true when there are no Critical or Warning findings.${where}`,
      { agentType: 'reviewer', label: `review:round${round}`, phase: 'Review', schema: FINDINGS_SCHEMA },
    ),
    () => round === 1
      ? agent(CODE_REVIEW_PROMPT + focus, { label: 'code-review', phase: 'Review', schema: CODE_REVIEW_SCHEMA })
      : Promise.resolve(null),
  ])
  if (round === 1) {
    // 実行の証拠を、コードで確かめる(プロンプトの指示だけに頼らない): 対象が worktree と一致し、差分が空でないこと
    const wrongTree = worktree && cr?.reviewedRoot && cr.reviewedRoot.replace(/\/$/, '') !== worktree.replace(/\/$/, '')
    const emptyDiff = cr?.ran && !(cr.diffFileCount > 0)
    codeReview = !cr?.ran ? { ran: false, reason: cr?.note || 'エージェントが失敗した、または結果を返さなかった' }
      : wrongTree ? { ran: false, reason: `別の木をレビューした(対象 ${cr.reviewedRoot}、期待 ${worktree})` }
      : emptyDiff ? { ran: false, reason: '対象の差分が空(何もレビューされていない)' }
      : { ran: true, reviewedRoot: cr.reviewedRoot, diffFileCount: cr.diffFileCount, findingCount: cr.findings.length, findings: cr.findings }
    // 黙って飛ばさない: 未実行なら、その理由と、手動で実行する依頼を、結果に必ず載せる
    if (!codeReview.ran) notices.push(`code-review 未実行(${codeReview.reason})。手動で /code-review を実行し、結果の要約を PR のコメントに残してください。`)
    else if (!worktree) notices.push(`code-review の対象は ${codeReview.reviewedRoot} でした(worktree を指定していません)。別の木を見ていないか、確かめてください。`)
  }
  if (!review) return result('aborted', { round })
  const crOk = codeReview.ran && round === 1
  const merged = mergeFindings([
    ...review.findings.map(f => ({ ...f, source: 'reviewer' })),
    ...(crOk ? cr.findings : []).map(f => ({ ...f, source: 'code-review' })),
  ])

  // Suggestion は V1 に通さず、自動修正もせず、未検証のまま提示する(ラウンドをまたいで溜める。上書きすると、1 ラウンド目の code-review の Suggestion が消える)
  suggestions.push(...merged.filter(f => f.severity === 'Suggestion'))
  const actionable = merged.filter(f => f.severity !== 'Suggestion')
  if (actionable.length === 0) {
    history.push({ round, actionable: 0 })
    return result('clean', { rounds: round })
  }

  // V1: verify every actionable finding in parallel
  const verified = await parallel(actionable.map(f => () =>
    agent(
      `Adversarially verify this single review finding against the actual code. Read the real code, trace guards and call paths, and try to construct the concrete failure. Judge autoFixSafe per your instructions.

Finding: ${JSON.stringify(f)}
If the finding proposes no fix ("no proposed fix"), it is not autoFixSafe unless the correct fix is obvious and local.${where}`,
      { agentType: 'verifier', label: `verify:${f.file}`, phase: 'Verify', schema: VERDICT_SCHEMA },
    ).then(v => ({ finding: f, verdict: v }))
  ))

  // V2: triage
  const autoFix = []
  for (const { finding, verdict } of verified) {
    if (!verdict) {
      // verifier が失敗した指摘は、黙って捨てず、利用者の判断(手動での確認)に回す
      userDecisions.push({ ...finding, verdict: 'UNCERTAIN', evidence: 'verifier が失敗した(結果なし)', decisionQuestion: `検証に失敗した指摘です。手動で確認しますか? ${finding.reason}`, priority: 'P2' })
      notices.push(`verifier が失敗: ${finding.file}`)
      continue
    }
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
Report per finding: fixed / skipped (with reason), files touched, and validation commands run with pass/fail.${where}`,
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
