import { describe, it, expect } from "vitest";
import { signupSchema } from "./useAuthForm";

const valid = {
  username: "taro",
  email: "taro@example.com",
  password: "Passw0rd!",
  passwordConfirmation: "Passw0rd!",
};

// エラーを path の先頭のフィールド名ごとに集める
function fieldErrors(input: unknown): Record<string, string[]> {
  const result = signupSchema.safeParse(input);
  if (result.success) return {};
  const errors: Record<string, string[]> = {};
  for (const issue of result.error.issues) {
    const field = String(issue.path[0]);
    (errors[field] ??= []).push(issue.message);
  }
  return errors;
}

describe("signupSchema", () => {
  it("有効な入力は通る", () => {
    expect(signupSchema.safeParse(valid).success).toBe(true);
  });

  it("パスワードが空なら、入力の有無だけを理由にエラーにする", () => {
    const errors = fieldErrors({ ...valid, password: "", passwordConfirmation: "" });
    expect(errors.password).toEqual(["Password is required"]);
  });

  it("パスワードの強度は frontend では判定しない(規則の判定は backend だけが持つ)", () => {
    // 短く、記号もない値でも、frontend のスキーマは通す。違反はサーバーの 422 で表示される
    const weak = { ...valid, password: "abc", passwordConfirmation: "abc" };
    expect(signupSchema.safeParse(weak).success).toBe(true);
  });

  it("確認欄が一致しなければ、確認欄のフィールドにエラーが出る", () => {
    const errors = fieldErrors({ ...valid, passwordConfirmation: "different" });
    expect(errors).toEqual({ passwordConfirmation: ["Passwords don't match"] });
  });
});
