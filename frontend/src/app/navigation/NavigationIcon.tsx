export function NavigationIcon({ kind }: { kind: 'home' | 'discover' | 'record' | 'profile' }) {
  return <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
    {kind === 'home' && <path d="M3 10 12 3 21 10 21 20 15 20 15 13 9 13 9 20 3 20Z" />}
    {kind === 'discover' && <><circle cx="10.5" cy="10.5" r="6.5" /><path d="m16 16 5 5" /></>}
    {kind === 'profile' && <><circle cx="12" cy="8" r="4" /><path d="M4 21v-2c0-10.67 16-10.67 16 0v2" /></>}
    {kind === 'record' && <>
      {/* 採用Penpotの白いバーガーと＋。輪郭を途切れさせ、＋と接続しない隙間を保つ。 */}
      <path d="M4 12C4 6.8 7.6 3 12 3c4.4 0 8 3.8 8 9v4c0 2.2-1.2 4-3 4H7c-1.8 0-3-1.8-3-4Z" fill="white" stroke="none" />
      <path d="M19.85 10.1C19.95 10.7 20 11.3 20 12v4c0 2.2-1.2 4-3 4H7c-1.8 0-3-1.8-3-4v-4c0-5.2 3.6-9 8-9 1.1 0 2.1.24 3 .65" />
      <path d="M4 12c1.2 0 1.5 1.5 3 1.5S9 12 10.5 12s2 1.5 3.5 1.5S16 12 17.5 12s1.7 1 2.5 0M4 16h16M17 4v6M14 7h6" strokeWidth="1.5" />
    </>}
  </svg>
}
