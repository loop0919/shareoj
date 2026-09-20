import { z } from 'zod'
import { difficultySchema } from './difficulty'
export const publicProblemSummarySchema = z.object({ id: z.string().uuid(), title: z.string().min(1), author: z.string(), difficulty: difficultySchema, solverCount: z.number().int().nonnegative().default(0), favoriteCount: z.number().int().nonnegative().default(0), timeLimitMs: z.coerce.number().int().positive().optional(), memoryLimitMb: z.coerce.number().int().positive().optional(), publishedAt: z.string().datetime({ offset: true }) })
export const publicProblemSchema = publicProblemSummarySchema.extend({ difficultyAverage: z.number().min(1).max(10).nullable().default(null), difficultyVoteCount: z.number().int().nonnegative().default(0), testers: z.array(z.string()).default([]), specialJudge: z.boolean().default(false), interactive: z.boolean().default(false), markdown: z.string().min(1), editorial: z.string().default(''), timeLimitMs: z.coerce.number().int().positive(), memoryLimitMb: z.coerce.number().int().positive() })
export const publicProblemListSchema = z.object({ items: z.array(publicProblemSummarySchema), nextCursor: z.string() })
export type PublicProblem = z.infer<typeof publicProblemSchema>
