import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

const createReviewSchema = z.object({
  rating: z.number().int().min(1).max(5),
  comment: z.string().min(1, "Comment is required"),
  shopId: z.number().int().positive("Shop is required"),
  burgerName: z.string().min(1, "Burger name is required"),
});

const updateReviewSchema = z.object({
  rating: z.number().int().min(1).max(5),
  comment: z.string().min(1, "Comment is required"),
});

export type CreateReviewFormData = z.infer<typeof createReviewSchema>;
export type UpdateReviewFormData = z.infer<typeof updateReviewSchema>;

export function useCreateReviewForm(defaults?: Partial<CreateReviewFormData>) {
  return useForm<CreateReviewFormData>({
    resolver: zodResolver(createReviewSchema),
    defaultValues: {
      rating: 3,
      comment: "",
      shopId: 0,
      burgerName: "",
      ...defaults,
    },
  });
}

export function useUpdateReviewForm(defaults?: Partial<UpdateReviewFormData>) {
  return useForm<UpdateReviewFormData>({
    resolver: zodResolver(updateReviewSchema),
    defaultValues: {
      rating: 3,
      comment: "",
      ...defaults,
    },
  });
}
