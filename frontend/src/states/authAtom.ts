import { atom } from "jotai";
import type { AuthUser } from "../domains/auth/types";

export const authUserAtom = atom<AuthUser | null>(null);
export const authTokenAtom = atom<string | null>(null);
