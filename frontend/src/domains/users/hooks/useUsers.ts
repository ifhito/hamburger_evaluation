import useSWR from "swr";
import { useSWRConfig } from "swr";
import { userApiClient } from "../api/userApiClient";
import type { User, UserUpdateInput } from "../api/types";

export function useUsers() {
  return useSWR<User[]>("/users", async (url: string) => {
    const res = await userApiClient.get<User[]>(url);
    if (!Array.isArray(res.data)) {
      throw new Error("Invalid response: expected array");
    }
    return res.data;
  });
}

export function useUpdateUser(id: number) {
  const { mutate } = useSWRConfig();
  return {
    update: async (data: UserUpdateInput): Promise<User> => {
      const res = await userApiClient.put<User>(`/users/${id}`, { user: data });
      await mutate("/users");
      return res.data;
    },
  };
}

export function useDeleteUser() {
  const { mutate } = useSWRConfig();
  return {
    destroy: async (id: number): Promise<void> => {
      await userApiClient.delete(`/users/${id}`);
      await mutate("/users");
    },
  };
}
