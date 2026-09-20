import { useForm } from "react-hook-form";

// 入力の検証は backend の domain だけが判定する。ここでは判定を持たず、違反はサーバーの 422 のメッセージで表示する。
export type ShopFormData = {
  name: string;
};

export function useShopForm(defaults?: Partial<ShopFormData>) {
  return useForm<ShopFormData>({
    defaultValues: { name: "", ...defaults },
  });
}
