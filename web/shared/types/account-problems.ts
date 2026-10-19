import { z } from 'zod'
import { problemDraftSchema } from './problem-draft'

// Per destination, the server's reasons why the saved problem cannot go there; empty means it can.
export const readinessSchema = z.object({ publish: z.array(z.string()), contest: z.array(z.string()), featured: z.array(z.string()) })
export type Readiness = z.infer<typeof readinessSchema>

// Setters of the published contest holding the problem edit it like testers until the results are final (ADR 0022).
const contestEditorsSchema = z.object({ contestId: z.string(), title: z.string(), setters: z.array(z.string()) })
// A tested problem the viewer manages only as a setter of this contest; author is the problem's owner.
const contestRefSchema = z.object({ id: z.string(), title: z.string(), author: z.string().default('') })
export const accountProblemSchema = z.object({ scheduledAt: z.string().datetime({ offset: true }).optional(), readiness: readinessSchema.optional(), featuredPreference: z.enum(['', 'soon', 'later']).default(''), contestScheduled: z.boolean().default(false), everPublished: z.boolean().default(false), testers: z.array(z.string()).default([]), contestEditors: contestEditorsSchema.optional(), contestId: z.string().default(''), author: z.string().default(''), id: z.string().uuid(), publishedVersion: z.number().int().nonnegative().default(0), version: z.number().int().positive(), updatedAt: z.string().datetime({ offset: true }), draft: problemDraftSchema })
export type AccountProblem = z.infer<typeof accountProblemSchema>
export const accountListSchema = z.object({
  items: z.array(z.object({ scheduledAt: z.string().datetime({ offset: true }).optional(), readiness: readinessSchema.optional(), featuredPreference: z.enum(['', 'soon', 'later']).default(''), contestScheduled: z.boolean().default(false), everPublished: z.boolean().default(false), contestId: z.string().default(''), viaContest: contestRefSchema.optional(), partial: z.boolean().default(false), fullScore: z.number().int().nonnegative().default(0), id: z.string().uuid(), title: z.string(), difficulty: z.number().int().min(1).max(10).nullable().default(null), publishedVersion: z.number().int().nonnegative().default(0), updatedAt: z.string().datetime({ offset: true }) })),
  nextCursor: z.string(),
})
export type AccountSummary = z.infer<typeof accountListSchema>['items'][number]

