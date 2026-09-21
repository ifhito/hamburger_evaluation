-- 統計の再計算の依頼。この表は、「このバーガーの統計を、計算し直す必要がある」という依頼を
-- 溜めておく待ち行列(順番待ちの列)である。レビューの投稿・編集・削除や退会をするとき、書き込みと
-- 同じトランザクションで、影響するバーガーの依頼を登録するだけで、統計は計算しない。バックグラウンドの
-- ワーカーが、依頼を取り出して統計を計算し直し、終えた依頼を消す。
-- 1 つのバーガーにつき 1 行で、同じバーガーへの書き込みが続いても、再計算は 1 回にまとまる。
--
-- version は、登録のたびに、時間をまたいで単調に増える番号(専用の sequence から取る)。
-- ワーカーは、取り出したときの version と一致する場合だけ行を消す。行を消して作り直したときに
-- 同じ番号に戻ると、古い再計算が、あとから入った依頼を消してしまうため、行ごとの +1 ではなく
-- sequence にしている。
-- attempts は失敗の回数、next_attempt_at は次に再試行する時刻(null は「すぐ」)、
-- last_error は直近の失敗の理由(切り詰めた文字列。秘密を含めない)。
CREATE SEQUENCE burger_stats_recalc_requests_version_seq;

CREATE TABLE burger_stats_recalc_requests (
    burger_id uuid PRIMARY KEY REFERENCES burgers (id),
    version bigint NOT NULL DEFAULT nextval('burger_stats_recalc_requests_version_seq'),
    attempts integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT burger_stats_recalc_requests_attempts_check CHECK (attempts >= 0),
    CONSTRAINT burger_stats_recalc_requests_last_error_max_length CHECK (char_length(last_error) <= 500)
);

-- 表を消すと sequence も一緒に消えるようにする。
ALTER SEQUENCE burger_stats_recalc_requests_version_seq OWNED BY burger_stats_recalc_requests.version;
