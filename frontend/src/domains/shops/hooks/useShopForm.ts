import { useForm } from "react-hook-form";
import type { ShopCreateInput } from "../api/types";

// 入力の検証は backend の domain だけが判定する。ここでは判定を持たず、違反はサーバーの 422 のメッセージで表示する。
export type ShopFormData = {
  name: string;
  mapUrl: string;
  // select の値。"" は未選択。
  prefectureCode: string;
  city: string;
  streetAddress: string;
};

export function useShopForm(defaults?: Partial<ShopFormData>) {
  return useForm<ShopFormData>({
    defaultValues: { name: "", mapUrl: "", prefectureCode: "", city: "", streetAddress: "", ...defaults },
  });
}

// フォームの値を、送信の本文にする。都道府県が未選択なら null(未設定)を送る。
export function toShopInput(data: ShopFormData): ShopCreateInput {
  return { ...data, prefectureCode: data.prefectureCode === "" ? null : Number(data.prefectureCode) };
}
