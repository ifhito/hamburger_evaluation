import { buildApiClient } from "../../../api/client/buildApiClient";
import { getToken } from "../storage";
import type { SignupRequest, SignupResponse, LoginRequest, LoginResponse } from "../types";

export const authApiClient = buildApiClient(getToken);

export const authApi = {
  async signup(data: SignupRequest): Promise<SignupResponse> {
    const res = await authApiClient.post<SignupResponse>("/signup", data);
    return res.data;
  },
  async login(data: LoginRequest): Promise<LoginResponse> {
    const res = await authApiClient.post<LoginResponse>("/login", data);
    return res.data;
  },
  async logout(): Promise<{ message: string }> {
    const res = await authApiClient.post<{ message: string }>("/logout");
    return res.data;
  },
};
