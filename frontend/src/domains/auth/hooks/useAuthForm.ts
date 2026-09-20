import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { validatePassword } from "../../../lib/password";

const loginSchema = z.object({
  email: z.string().email("Invalid email address"),
  password: z.string().min(1, "Password is required"),
});

const signupSchema = z.object({
  username: z.string().min(1, "Username is required"),
  email: z.string().email("Invalid email address"),
  // 規則の判定は validatePassword に一本化(サーバーと同じメッセージ)。違反は ". " でつないで 1 つのエラーにする
  password: z.string().superRefine((value, ctx) => {
    const messages = validatePassword(value);
    if (messages.length > 0) ctx.addIssue({ code: "custom", message: messages.join(". ") });
  }),
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
