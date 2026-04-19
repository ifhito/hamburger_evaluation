import { useState } from "react";
import { useNavigate, useParams, Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { useUpdateUser, useDeleteUser } from "../hooks/useUsers";
import { ApiError } from "../../../api/client/buildApiClient";
import { Button } from "../../../components/Button";
import { ErrorMessage } from "../../../components/ErrorMessage";
import { Input } from "../../../components/Input";
import { Layout } from "../../../components/Layout";
import styles from "./userUpdate.module.css";

export default function UserUpdatePage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { user: authUser, logout, refreshUser } = useAuth();
  const { update } = useUpdateUser(Number(id));
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
      refreshUser({ id: updated.id, username: updated.username, email: updated.email });
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
      await destroy(Number(id));
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
      <form onSubmit={(e) => void handleUpdate(e)} className={styles.form}>
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
