import { useTranslation } from "react-i18next";
import { Badge } from "../../../components/ui/Badge";
import type { OAuthScope } from "../api/types";
import styles from "./scopeList.module.css";

// 許可の範囲の説明の並び(許可の画面・接続済みアプリの一覧で共通)。既知の範囲の説明を翻訳し、未知の範囲は API の説明をそのまま出す。
// 箇条書きの印は、実際の DOM の要素(aria-hidden の span)で付ける。CSS の ::before の文字は、一部の
// スクリーンリーダーが読み上げてしまうことがあるため使わない。
// 外側の余白・枠(囲むか、ただの並びか)は、置き場所ごとに違うので、呼び出し側の className で渡す。
// 表示用の訳語だけを対応付ける。許可の有無や書き込みの判定には使わない。
const scopeDescriptionKeys: Record<string, string> = {
  "hamburger:read": "oauth.scopeDescriptions.read",
  "hamburger:write": "oauth.scopeDescriptions.write",
  "hamburger:admin": "oauth.scopeDescriptions.admin",
};

export function ScopeList({ scopes, className }: { scopes: OAuthScope[]; className?: string }) {
  const { t } = useTranslation();
  return (
    <ul className={[styles.scopes, className].filter(Boolean).join(" ")}>
      {scopes.map((scope) => (
        <li key={scope.name} className={styles.scopeItem}>
          <span className={styles.scopeText}>
            <span aria-hidden="true">・</span>
            {Object.prototype.hasOwnProperty.call(scopeDescriptionKeys, scope.name) ? t(scopeDescriptionKeys[scope.name], { defaultValue: scope.description }) : scope.description}
          </span>
          {/* 書き込みの範囲かは、API の印(writes)だけで決める(範囲の名前を比べない) */}
          {scope.writes && <Badge tone="accent">{t("oauth.writeAccess")}</Badge>}
        </li>
      ))}
    </ul>
  );
}
