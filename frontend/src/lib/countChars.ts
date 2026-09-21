// 文字数は、backend と同じコードポイント数で数える(日本語も絵文字も 1 文字)。
// UTF-16 の length は、絵文字を 2 と数えて backend と食い違うので使わない。
export function countChars(value: string): number {
  return Array.from(value).length
}
