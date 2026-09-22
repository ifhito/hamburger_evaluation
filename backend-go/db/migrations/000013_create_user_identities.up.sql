-- 外部のサービス(Google など)のアカウントとの結び付き。利用者の識別には、メールアドレスではなく、
-- サービスが付ける、変わらない ID(provider_user_id。Google の `sub`)を使う。サービスのトークンは保存しない。
-- email は、結び付けたときのサービス側のメールで、画面に見せるためだけに持つ(サインインの判断には使わない)。
CREATE TABLE user_identities (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider text NOT NULL,
    provider_user_id text NOT NULL,
    email text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    -- provider の値は internal/domain の定数(ProviderGoogle)と同じでなければならない。サービスを足すときは、
    -- 定数とこの CHECK の両方に足す。
    CONSTRAINT user_identities_provider_check CHECK (provider IN ('google')),
    CONSTRAINT user_identities_provider_user_id_check CHECK (provider_user_id <> '' AND char_length(provider_user_id) <= 255),
    CONSTRAINT user_identities_email_max_length CHECK (char_length(email) <= 254),
    -- 同じ外部のアカウントを、複数のアカウントに結び付けない。
    CONSTRAINT user_identities_provider_user_key UNIQUE (provider, provider_user_id),
    -- 1 つのアカウントに、同じサービスは 1 つだけ結び付ける(この制約の索引は、user_id での検索にも使う)。
    CONSTRAINT user_identities_user_provider_key UNIQUE (user_id, provider)
);
