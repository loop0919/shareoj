import { z } from 'zod'
// A problem the post explains. The title is empty once the problem stops being public.
export const postProblemSchema = z.object({ id: z.string().uuid(), title: z.string() })
export const postSummarySchema = z.object({ id: z.string().uuid(), title: z.string(), publishedVersion: z.number().int().nonnegative(), publishedAt: z.string().datetime({ offset: true }).nullable(), updatedAt: z.string(), author: z.string().optional(), isOperator: z.boolean(), tags: z.array(z.string()).default([]) })
export const postSchema = postSummarySchema.extend({ markdown: z.string().default(''), version: z.number().int().positive(), problems: z.array(postProblemSchema).default([]) })
export const publicPostSchema = postSummarySchema.extend({ markdown: z.string().min(1), author: z.string(), publishedAt: z.string().datetime({ offset: true }), problems: z.array(postProblemSchema).default([]) })
export const postListSchema = z.object({ items: z.array(postSummarySchema), nextCursor: z.string() })
export const userEditorialListSchema = z.object({ items: z.array(z.object({ problemId: z.string().uuid(), id: z.string().uuid(), title: z.string(), author: z.string(), publishedAt: z.string().datetime({ offset: true }), isOperator: z.boolean() })) })
export type PostProblem = z.infer<typeof postProblemSchema>
export type UserEditorial = z.infer<typeof userEditorialListSchema>['items'][number]
