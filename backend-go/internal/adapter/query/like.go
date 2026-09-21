package query

import "strings"

// likeEscaper は LIKE のメタ文字をエスケープする（最初にバックスラッシュ）。
// これにより、user の keyword は ILIKE パターンの中で常にリテラルとして
// 一致する。
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
