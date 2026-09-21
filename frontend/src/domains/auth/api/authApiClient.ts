import { buildApiClient } from "../../../api/client/buildApiClient";
import { getToken } from "../storage";
import type {
  AuthUserResponse,
  LoginRequest,
  LoginResponse,
  SignupAcceptedResponse,
  SignupRequest,
} from "../types";

export const authApiClient = buildApiClient(getToken);

export const authApi = {
  async signup(data: SignupRequest): Promise<SignupAcceptedResponse> {
    const res = await authApiClient.post<SignupAcceptedResponse>("/signup", data);
    return res.data;
  },
  async confirmSignup(token: string): Promise<AuthUserResponse> {
    const res = await authApiClient.post<AuthUserResponse>("/signup/confirm", { token });
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
