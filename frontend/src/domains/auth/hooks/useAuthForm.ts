import { useForm } from "react-hook-form";

// 入力の検証(必須・email の形式・パスワードの規則・確認欄の一致)は backend の domain だけが判定する。
// ここでは判定を持たず、違反はサーバーの 422 のメッセージで表示する。
export type LoginFormData = {
  email: string;
  password: string;
};

export type SignupFormData = {
  username: string;
  email: string;
  password: string;
  passwordConfirmation: string;
};

export function useLoginForm() {
  return useForm<LoginFormData>({
    defaultValues: { email: "", password: "" },
  });
}

export function useSignupForm() {
  return useForm<SignupFormData>({
    defaultValues: { username: "", email: "", password: "", passwordConfirmation: "" },
  });
}
