// review-fix.js の自己テスト。`node .claude/workflows/review-fix.selftest.mjs` で動く(偽のエージェントで、ワークフローの分岐だけを確かめる)。
import { readFileSync } from 'node:fs'
import assert from 'node:assert/strict'

const src = readFileSync(new URL('./review-fix.js', import.meta.url), 'utf8').replace(/^export const meta/m, 'const meta')
const run = async (args, fake) => {
  const agent = async (prompt, opts) => fake(prompt, opts)
  const parallel = async (thunks) => Promise.all(thunks.map((t) => t()))
  return new Function('args', 'agent', 'parallel', 'log', `return (async () => {${src}})()`)(args, agent, parallel, () => {})
}
const f = (severity, file, reason = 'r') => ({ severity, file, reason, fix: 'x' })
const cr = (over = {}) => ({ ran: true, reviewedRoot: '/w', diffFileCount: 3, findings: [], ...over })

// 1. reviewer が失敗(null)しても、code-review の実行結果(codeReview)は失われない
let r = await run('worktree: /w', (_p, o) => (o.label === 'code-review' ? cr() : null))
assert.equal(r.status, 'aborted')
assert.equal(r.codeReview.ran, true)

// 2. 別の木をレビューした / 差分が空 / 実行できなかった → 未実行として、notices に理由を載せる
for (const [c, word] of [[cr({ reviewedRoot: '/main' }), '別の木'], [cr({ diffFileCount: 0 }), '差分が空'], [{ ran: false, note: 'boom', reviewedRoot: '', diffFileCount: 0, findings: [] }, 'boom']]) {
  r = await run('worktree: /w', (_p, o) => (o.label === 'code-review' ? c : { clean: true, findings: [] }))
  assert.equal(r.codeReview.ran, false)
  assert.ok(r.notices.join().includes(word), word)
}

// 3. worktree を指定しないと、対象を確かめるよう notices で促す
r = await run('', (_p, o) => (o.label === 'code-review' ? cr({ reviewedRoot: '/main' }) : { clean: true, findings: [] }))
assert.ok(r.notices.join().includes('worktree を指定していません'))

// 4. 別々のパスが同じ path:line を指したときだけ、まとめる。同じパスの別々の指摘は、まとめない
const seen = []
r = await run('worktree: /w', (p, o) => {
  if (o.label === 'code-review') return cr({ findings: [f('Critical', '/w/a.go:10-20', 'cr'), f('Warning', 'b.go:5', 'cr2'), f('Warning', 'b.go:5', 'cr3')] })
  if (o.agentType === 'reviewer') return { clean: false, findings: [f('Warning', 'a.go:10', 'rv'), f('Suggestion', 'c.go:1', 's1')] }
  if (o.agentType === 'verifier') { seen.push(p); return { verdict: 'CONFIRMED', evidence: 'e', autoFixSafe: false, priority: 'P2' } }
  return null
})
assert.equal(seen.length, 3, 'a.go(まとめる)1 + b.go 2 = 3 件が V1 に行く')
assert.ok(seen.some((p) => p.includes('reviewer+code-review')), '別々のパスの指摘はまとめる')
assert.equal(r.suggestions.length, 1, 'Suggestion は V1 に通さず、提示する')

// 5. verifier と reviewer に worktree のパスが伝わる
assert.ok(seen.every((p) => p.includes('git worktree /w')))

// 6. verifier が失敗(null)した指摘は、黙って捨てず、利用者の判断に回す。code-review が未実行なら complete=false
r = await run('worktree: /w', (_p, o) => {
  if (o.label === 'code-review') return { ran: false, note: 'x', reviewedRoot: '', diffFileCount: 0, findings: [] }
  if (o.agentType === 'reviewer') return { clean: false, findings: [f('Warning', 'a.go:1')] }
  return null
})
assert.equal(r.userDecisions.length, 1)
assert.equal(r.complete, false)

// 7. 1 ラウンド目の code-review の Suggestion は、2 ラウンド目で上書きされずに残る
let rounds = 0
r = await run('worktree: /w', (_p, o) => {
  if (o.label === 'code-review') return cr({ findings: [f('Suggestion', 'z.go:9', 'opinion')] })
  if (o.agentType === 'reviewer') return ++rounds === 1 ? { clean: false, findings: [f('Critical', 'a.go:1')] } : { clean: true, findings: [] }
  if (o.agentType === 'verifier') return { verdict: 'CONFIRMED', evidence: 'e', autoFixSafe: true }
  return 'fixed'
})
assert.equal(r.status, 'clean')
assert.equal(r.suggestions.length, 1)
assert.equal(r.complete, true)

console.log('review-fix.selftest: OK')
