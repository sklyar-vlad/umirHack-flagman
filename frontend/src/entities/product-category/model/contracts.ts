import { z } from 'zod'
export const categoryNameSchema = z.string().trim().refine((value) => [...value].length >= 1 && [...value].length <= 200, 'Введите название от 1 до 200 символов.')
export const categorySchema = z.object({ id: z.uuid(), name: z.string().min(1), revision: z.number().int().positive().max(Number.MAX_SAFE_INTEGER) })
export const categoryListSchema = z.object({ items: z.array(categorySchema), total: z.number().int().nonnegative(), page: z.number().int().positive(), page_size: z.number().int().min(1).max(100), meta: z.object({}).passthrough() })
export const createCategorySchema = z.object({ name: categoryNameSchema }).strict()
export const updateCategorySchema = createCategorySchema.extend({ expected_revision: z.number().int().min(1).max(2147483646) })
export type Category = z.infer<typeof categorySchema>
export type CategoryList = z.infer<typeof categoryListSchema>
export type CreateCategory = z.infer<typeof createCategorySchema>
export type UpdateCategory = z.infer<typeof updateCategorySchema>
export interface CategorySearch { search: string; sort: 'name'; page: number; page_size: number }
