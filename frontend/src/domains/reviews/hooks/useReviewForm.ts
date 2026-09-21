import { useForm } from "react-hook-form";

// 入力の検証(必須・rating の範囲・shop の指定)は backend の domain だけが判定する。
// ここでは判定を持たず、違反はサーバーの 422 のメッセージで表示する。
export type CreateReviewFormData = {
  rating: number;
  comment: string;
  shopId: string;
  burgerName: string;
};

export type UpdateReviewFormData = {
  rating: number;
  comment: string;
};

export function useCreateReviewForm(defaults?: Partial<CreateReviewFormData>) {
  return useForm<CreateReviewFormData>({
    defaultValues: {
      rating: 3,
      comment: "",
      shopId: "",
      burgerName: "",
      ...defaults,
    },
  });
}

export function useUpdateReviewForm(defaults?: Partial<UpdateReviewFormData>) {
  return useForm<UpdateReviewFormData>({
    defaultValues: {
      rating: 3,
      comment: "",
      ...defaults,
    },
  });
}
