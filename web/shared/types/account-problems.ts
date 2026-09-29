import { z } from 'zod'
import { problemDraftSchema } from './problem-draft'

// Per destination, the server's reasons why the saved problem cannot go there; empty means it can.
export const readinessSchema = z.object({ publish: z.array(z.string()), contest: z.array(z.string()), featured: z.array(z.string()) })
export type Readiness = z.infer<typeof readinessSchema>

export const accountProblemSchema = z.object({ readiness: readinessSchema.optional(), featuredPreference: z.enum(['', 'soon', 'later']).default(''), contestScheduled: z.boolean().default(false), everPublished: z.boolean().default(false), testers: z.array(z.string()).default([]), contestId: z.string().default(''), author: z.string().default(''), id: z.string().uuid(), publishedVersion: z.number().int().nonnegative().default(0), version: z.number().int().positive(), updatedAt: z.string().datetime({ offset: true }), draft: problemDraftSchema })
export const accountListSchema = z.object({
  items: z.array(z.object({ readiness: readinessSchema.optional(), featuredPreference: z.enum(['', 'soon', 'later']).default(''), contestScheduled: z.boolean().default(false), everPublished: z.boolean().default(false), contestId: z.string().default(''), id: z.string().uuid(), title: z.string(), publishedVersion: z.number().int().nonnegative().default(0), updatedAt: z.string().datetime({ offset: true }) })),
  nextCursor: z.string(),
})
export type AccountSummary = z.infer<typeof accountListSchema>['items'][number]

