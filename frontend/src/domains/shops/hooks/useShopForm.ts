import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

const shopSchema = z.object({
  name: z.string().min(1, "Name is required"),
});

export type ShopFormData = z.infer<typeof shopSchema>;

export function useShopForm(defaults?: Partial<ShopFormData>) {
  return useForm<ShopFormData>({
    resolver: zodResolver(shopSchema),
    defaultValues: { name: "", ...defaults },
  });
}
