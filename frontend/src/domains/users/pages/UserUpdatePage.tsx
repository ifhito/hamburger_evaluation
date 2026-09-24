import { useState, type ReactNode } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { useUpdateUser, useDeleteUser } from "../hooks/useUserMutations";
import { useUser } from "../hooks/useUser";
import { ApiError } from "../../../api/client/buildApiClient";
import { useMeta } from "../../../api/meta";
import { Alert } from "../../../components/ui/Alert";
import { Button } from "../../../components/ui/Button";
import { LinkButton } from "../../../components/ui/LinkButton";
import { Loading } from "../../../components/ui/states";
import { TextField, TextArea } from "../../../components/ui/TextField";
import { TextLink } from "../../../components/ui/TextLink";
import { Layout } from "../../../components/Layout";
import styles from "./userUpdate.module.css";

// 編集の画面の枠(戻るリンク・見出し)。どの状態(読み込み中・失敗・編集できない・フォーム)でも同じ。
function EditColumn({ id, children }: { id: string | undefined; children: ReactNode }) {
  const { t } = useTranslation();
  return (
    <Layout>
      <div className={styles.column}>
        <TextLink to={`/users/${id}`}>{t("users.update.backToProfile")}</TextLink>
        <h1 className={styles.title}>{t("users.update.title")}</h1>
        {children}
      </div>
    </Layout>
  );
}

// 編集フォーム本体。編集してよい(backend の canEdit が true の)ときだけ、UserUpdatePage が表示する。
// 初期値は、取得済みの profile(initialUsername/initialEmail/initialBio)から取る。AuthProvider の authUser は
// ログイン直後のキャッシュで、他の画面での更新後に古いままのことがあるため、初期値にも変更の判定にも使わない。
function UserUpdateForm({
  id,
  initialUsername,
  initialBio,
  initialEmail,
}: {
  id: string;
  initialUsername: string;
  initialBio: string;
  initialEmail: string;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { logout, refreshUser } = useAuth();
  const { update } = useUpdateUser(id);
  const { destroy } = useDeleteUser();
  const meta = useMeta().data;

  const [username, setUsername] = useState(initialUsername);
  const [bio, setBio] = useState(initialBio);
  const [email, setEmail] = useState(initialEmail);
  const [password, setPassword] = useState("");
  const [passwordConfirmation, setPasswordConfirmation] = useState("");
  // 失敗の見出しは画面の言葉、文言(messages)は API が返したものをそのまま出す
  const [serverError, setServerError] = useState<{ title: string; messages: string | string[] } | null>(null);
  const [isUpdating, setIsUpdating] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);

  const handleUpdate = async (e: React.FormEvent) => {
    e.preventDefault();
    setServerError(null);
    const data: Record<string, string> = {};
    if (username !== initialUsername) data.username = username;
    // 自己紹介文の上限などの規則は backend だけが判定し、違反はサーバーの 422 のメッセージで表示する。
    // 空文字も「書いた内容を消す」操作なので、そのまま送る
    if (bio !== initialBio) data.bio = bio;
    if (email !== initialEmail) data.email = email;
    if (password) {
      // 空のときは変更しない。規則の判定は backend だけが持ち、違反はサーバーの 422 のメッセージで表示する
      data.password = password;
      data.passwordConfirmation = passwordConfirmation;
    }
    if (Object.keys(data).length === 0) {
      void navigate(`/users/${id}`);
      return;
    }
    setIsUpdating(true);
    try {
      const updated = await update(data);
      refreshUser({ username: updated.username, email: updated.email });
      void navigate(`/users/${id}`);
    } catch (e) {
      setServerError({
        title: t("users.update.updateErrorTitle"),
        messages: e instanceof ApiError ? e.messages : [t("users.update.updateError")],
      });
    } finally {
      setIsUpdating(false);
    }
  };

  const handleDelete = async () => {
    if (!confirm(t("users.update.deleteConfirm"))) return;
    setIsDeleting(true);
    try {
      await destroy(id);
      await logout();
      void navigate("/reviews");
    } catch (e) {
      setServerError({
        title: t("users.update.deleteErrorTitle"),
        messages: e instanceof ApiError ? e.messages : [t("users.update.deleteError")],
      });
    } finally {
      setIsDeleting(false);
    }
  };

  return (
    <EditColumn id={id}>
      <form onSubmit={(e) => void handleUpdate(e)} className={styles.form} noValidate>
        {serverError && <Alert title={serverError.title} message={serverError.messages} />}
        <TextField
          id="username"
          label={t("users.update.username")}
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          counter={{ value: username, max: meta?.text.usernameMaxChars }}
          autoComplete="username"
        />
        <TextArea
          id="bio"
          label={t("users.update.bio")}
          value={bio}
          onChange={(e) => setBio(e.target.value)}
          counter={{ value: bio, max: meta?.text.bioMaxChars }}
          rows={5}
        />
        <TextField
          id="email"
          label={t("users.update.email")}
          type="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          autoComplete="email"
        />
        <TextField
          id="password"
          label={t("users.update.newPassword")}
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          autoComplete="new-password"
          hint={meta && t("auth.passwordHint", { min: meta.password.minBytes, max: meta.password.maxBytes })}
        />
        <TextField
          id="passwordConfirmation"
          label={t("users.update.confirmPassword")}
          type="password"
          value={passwordConfirmation}
          onChange={(e) => setPasswordConfirmation(e.target.value)}
          autoComplete="new-password"
        />
        <div className={styles.actions}>
          <Button type="submit" wide isLoading={isUpdating}>
            {t("users.update.submit")}
          </Button>
          <LinkButton to={`/users/${id}`}>{t("users.update.cancel")}</LinkButton>
        </div>
      </form>

      <div className={styles.divider} />
      <section>
        <h2 className={styles.dangerHeading}>{t("users.update.deleteHeading")}</h2>
        <p className={styles.dangerText}>{t("users.update.deleteWarning")}</p>
        <Button variant="danger" isLoading={isDeleting} onClick={() => void handleDelete()}>
          {t("users.update.deleteAccount")}
        </Button>
      </section>
    </EditColumn>
  );
}

// 編集できるかは backend が返す(canEdit)。他人の id のときは、自分の値を表示せず、編集できない旨を表示する。
export default function UserUpdatePage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const { user: authUser, isLoading: authLoading } = useAuth();
  const { data: profile, error, isLoading } = useUser(id, authUser?.id ?? null, {
    enabled: !authLoading,
  });

  if (authLoading || isLoading) {
    return (
      <EditColumn id={id}>
        <Loading />
      </EditColumn>
    );
  }
  if (error || !profile) {
    return (
      <EditColumn id={id}>
        <Alert message={t("users.detail.loadError")} />
      </EditColumn>
    );
  }
  if (!profile.canEdit) {
    return (
      <EditColumn id={id}>
        <Alert message={t("users.update.forbidden")} />
        <TextLink to={`/users/${id}`} className={styles.below}>
          {t("users.update.backToProfile")}
        </TextLink>
      </EditColumn>
    );
  }
  return (
    <UserUpdateForm
      id={profile.id}
      initialUsername={profile.username}
      initialBio={profile.bio}
      initialEmail={profile.email ?? ""}
    />
  );
}
