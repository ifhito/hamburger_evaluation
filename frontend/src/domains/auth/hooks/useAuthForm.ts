import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

const loginSchema = z.object({
  email: z.string().email("Invalid email address"),
  password: z.string().min(1, "Password is required"),
});

export const signupSchema = z.object({
  username: z.string().min(1, "Username is required"),
  email: z.string().email("Invalid email address"),
  // パスワードの規則(長さ・文字種)は backend の domain だけが判定する。ここでは入力の有無だけを確かめ、
  // 規則違反はサーバーの 422 のメッセージで表示する
  password: z.string().min(1, "Password is required"),
  passwordConfirmation: z.string().min(1, "Please confirm your password"),
}).refine((data) => data.password === data.passwordConfirmation, {
  message: "Passwords don't match",
  path: ["passwordConfirmation"],
});

export type LoginFormData = z.infer<typeof loginSchema>;
export type SignupFormData = z.infer<typeof signupSchema>;

export function useLoginForm() {
  return useForm<LoginFormData>({
    resolver: zodResolver(loginSchema),
    defaultValues: { email: "", password: "" },
  });
}

export function useSignupForm() {
  return useForm<SignupFormData>({
    resolver: zodResolver(signupSchema),
    defaultValues: { username: "", email: "", password: "", passwordConfirmation: "" },
  });
}
