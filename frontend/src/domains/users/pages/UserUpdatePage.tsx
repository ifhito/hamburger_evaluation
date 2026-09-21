import { useState } from "react";
import { useNavigate, useParams, Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { useUpdateUser, useDeleteUser } from "../hooks/useUserMutations";
import { useUser } from "../hooks/useUser";
import { ApiError } from "../../../api/client/buildApiClient";
import { Button } from "../../../components/Button";
import { ErrorMessage } from "../../../components/ErrorMessage";
import { Input } from "../../../components/Input";
import { Layout } from "../../../components/Layout";
import styles from "./userUpdate.module.css";

// 編集フォーム本体。編集してよい(backend の canEdit が true の)ときだけ、UserUpdatePage が表示する。
function UserUpdateForm({ id }: { id: string }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { user: authUser, logout, refreshUser } = useAuth();
  const { update } = useUpdateUser(id);
  const { destroy } = useDeleteUser();

  const [username, setUsername] = useState(authUser?.username ?? "");
  const [email, setEmail] = useState(authUser?.email ?? "");
  const [password, setPassword] = useState("");
  const [passwordConfirmation, setPasswordConfirmation] = useState("");
  const [serverError, setServerError] = useState<string | string[] | null>(null);
  const [isUpdating, setIsUpdating] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);

  const handleUpdate = async (e: React.FormEvent) => {
    e.preventDefault();
    setServerError(null);
    const data: Record<string, string> = {};
    if (username !== authUser?.username) data.username = username;
    if (email !== authUser?.email) data.email = email;
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
      setServerError(e instanceof ApiError ? e.messages : [t("users.update.updateError")]);
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
      setServerError(e instanceof ApiError ? e.messages : [t("users.update.deleteError")]);
    } finally {
      setIsDeleting(false);
    }
  };

  return (
    <Layout title={t("users.update.title")}>
      <form onSubmit={(e) => void handleUpdate(e)} className={styles.form} noValidate>
        {serverError && <ErrorMessage message={serverError} />}
        <Input
          id="username"
          label={t("users.update.username")}
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          autoComplete="username"
        />
        <Input
          id="email"
          label={t("users.update.email")}
          type="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          autoComplete="email"
        />
        <Input
          id="password"
          label={t("users.update.newPassword")}
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          autoComplete="new-password"
          hint={t("auth.passwordHint")}
        />
        <Input
          id="passwordConfirmation"
          label={t("users.update.confirmPassword")}
          type="password"
          value={passwordConfirmation}
          onChange={(e) => setPasswordConfirmation(e.target.value)}
          autoComplete="new-password"
        />
        <div className={styles.actions}>
          <Button type="submit" isLoading={isUpdating}>
            {t("users.update.submit")}
          </Button>
          <Link to={`/users/${id}`}>
            <Button type="button" variant="secondary">
              {t("users.update.cancel")}
            </Button>
          </Link>
        </div>
      </form>

      <hr className={styles.divider} />
      <div>
        <h3 className={styles.dangerZone}>{t("users.update.dangerZone")}</h3>
        <Button
          variant="danger"
          isLoading={isDeleting}
          onClick={() => void handleDelete()}
        >
          {t("users.update.deleteAccount")}
        </Button>
      </div>
    </Layout>
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
      <Layout title={t("users.update.title")}>
        <p className={styles.muted}>{t("users.detail.loading")}</p>
      </Layout>
    );
  }
  if (error || !profile) {
    return (
      <Layout title={t("users.update.title")}>
        <ErrorMessage message={t("users.detail.loadError")} />
      </Layout>
    );
  }
  if (!profile.canEdit) {
    return (
      <Layout title={t("users.update.title")}>
        <ErrorMessage message={t("users.update.forbidden")} />
        <Link to={`/users/${id}`}>{t("users.update.backToProfile")}</Link>
      </Layout>
    );
  }
  return <UserUpdateForm id={profile.id} />;
}
