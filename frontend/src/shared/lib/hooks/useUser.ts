import { useQuery } from '@tanstack/react-query'
import { usersApi } from '../api'

// viewerId は API の返す内容（本人にだけ email が付く）が閲覧者ごとに違うためキーに含める。
// ログアウトや別ユーザーでのログイン後に、前の閲覧者向けのキャッシュ（email 入り）を再利用しない。
// ['users'] 配下なので useUserMutations の invalidateQueries({ queryKey: ['users'] }) で更新される。
// options.enabled: viewerId がまだ確定していない間（認証状態の復元前）は false にする。
// token は localStorage にあるのに viewerId が null のまま取得すると、本人の email 入りの応答が匿名キーに入ってしまう。
export function useUser(id: number, viewerId: number | null, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: ['users', id, viewerId],
    queryFn: () => usersApi.get(id),
    // NaN（/users/abc）や小数の id では取得しない
    enabled: Number.isInteger(id) && (options?.enabled ?? true),
  })
}
