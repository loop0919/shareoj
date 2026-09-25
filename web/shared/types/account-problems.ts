import { z } from 'zod'
import { problemDraftSchema } from './problem-draft'

export const accountProblemSchema = z.object({ testers: z.array(z.string()).default([]), contestId: z.string().default(''), author: z.string().default(''), id: z.string().uuid(), publishedVersion: z.number().int().nonnegative().default(0), version: z.number().int().positive(), updatedAt: z.string().datetime({ offset: true }), draft: problemDraftSchema })
export const accountListSchema = z.object({
  items: z.array(z.object({ featuredPreference: z.enum(['', 'soon', 'later']).default(''), contestScheduled: z.boolean().default(false), everPublished: z.boolean().default(false), contestId: z.string().default(''), id: z.string().uuid(), title: z.string(), publishedVersion: z.number().int().nonnegative().default(0), updatedAt: z.string().datetime({ offset: true }) })),
  nextCursor: z.string(),
})
export type AccountSummary = z.infer<typeof accountListSchema>['items'][number]

