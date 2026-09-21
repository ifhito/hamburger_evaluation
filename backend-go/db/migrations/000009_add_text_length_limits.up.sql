-- S21: テキスト入力の文字数の上限(コードポイント数。char_length と同じ数え方)を DB でも守る。
-- 上限の値は internal/domain/limits.go の定数と同じでなければならない
-- (食い違いは db/migrations_test.go が検出する)。
--
-- 既存の行が上限を超えているときは、自動で切り詰めず、該当の行を報告して止める。
-- この DO ブロックと、下の ALTER は、同じ 1 つの transaction で実行される(止まったら何も変わらない)。
DO $$
DECLARE
    spec record;
    offenders text;
BEGIN
    FOR spec IN
        SELECT * FROM (VALUES
            ('reviews', 'comment', 2000),
            ('burgers', 'name', 100),
            ('shops', 'name', 100),
            ('shops', 'moderation_note', 500),
            ('users', 'username', 50),
            ('users', 'email', 254)
        ) AS v (tbl, col, max_chars)
    LOOP
        EXECUTE format(
            'SELECT string_agg(format(''id=%%s (%%s chars)'', id, char_length(%2$I)), '', '' ORDER BY id)
               FROM (SELECT id, %2$I FROM %1$I WHERE char_length(%2$I) > %3$s ORDER BY id LIMIT 20) t',
            spec.tbl, spec.col, spec.max_chars)
        INTO offenders;
        IF offenders IS NOT NULL THEN
            RAISE EXCEPTION 'Cannot add the length limit: %.% has rows over % characters (up to 20 shown): %',
                spec.tbl, spec.col, spec.max_chars, offenders;
        END IF;
    END LOOP;
END
$$;

ALTER TABLE reviews ADD CONSTRAINT reviews_comment_max_length CHECK (char_length(comment) <= 2000);
ALTER TABLE burgers ADD CONSTRAINT burgers_name_max_length CHECK (char_length(name) <= 100);
ALTER TABLE shops ADD CONSTRAINT shops_name_max_length CHECK (char_length(name) <= 100);
ALTER TABLE shops ADD CONSTRAINT shops_moderation_note_max_length CHECK (char_length(moderation_note) <= 500);
ALTER TABLE users ADD CONSTRAINT users_username_max_length CHECK (char_length(username) <= 50);
ALTER TABLE users ADD CONSTRAINT users_email_max_length CHECK (char_length(email) <= 254);
