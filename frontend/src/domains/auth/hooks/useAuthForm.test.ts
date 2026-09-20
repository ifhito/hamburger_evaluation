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

  it("パスワードが複数の規則に違反しても、password のエラーは 1 件で、違反が \". \" でつながる", () => {
    // "abc123" は短く(6 バイト)、記号もない
    const errors = fieldErrors({ ...valid, password: "abc123", passwordConfirmation: "abc123" });
    expect(errors).toEqual({
      password: [
        "Password is too short (minimum is 8 characters). Password must include letters, numbers and symbols",
      ],
    });
  });

  it("パスワードの違反と確認欄の不一致は、別のフィールドに両方出る", () => {
    const errors = fieldErrors({ ...valid, password: "abc123", passwordConfirmation: "different" });
    expect(errors).toEqual({
      password: [
        "Password is too short (minimum is 8 characters). Password must include letters, numbers and symbols",
      ],
      passwordConfirmation: ["Passwords don't match"],
    });
  });
});
