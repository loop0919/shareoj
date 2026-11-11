import { z } from 'zod'
import { difficultySchema } from './difficulty'

export const testFileLimit = 16 << 20
export const inlineTestDataLimit = 64 << 10
export const inlineTestSetLimit = 256 << 10
export const testSetLimit = 512 << 20
export const testFileSchema = z.object({ id: z.string().uuid(), size: z.number().int().positive().max(testFileLimit), sha256: z.string().regex(/^[a-f0-9]{64}$/) })
export type TestFile = z.infer<typeof testFileSchema>
export const generatorSchema = z.object({ runtime: z.string().max(64).default('cpp23-gcc'), source: z.string().default('') })
const judgeCodeSchema = generatorSchema.extend({ protocol: z.enum(['legacy', 'testlib']).optional() }).refine(
  code => code.protocol !== 'testlib' || ['cpp23-gcc', 'cpp23-clang'].includes(code.runtime),
  { message: 'testlib形式ではC++23のGCCまたはClangを選んでください。' },
)
// The error judge compares the numeric tokens of the output within an error and carries no code. It accepts an error up
// to ε × (1 + m/100), where m is the margin in percent of ε: 0.1 when marginPercent is absent.
export const errorJudgeModes = ['absolute', 'relative', 'either'] as const
export const errorJudgeEpsilons = ['1e-1', '1e-2', '1e-3', '1e-4', '1e-5', '1e-6', '1e-7', '1e-8', '1e-9', '1e-10', '1e-11', '1e-12'] as const
// From 0 to 100 with at most six decimals, as the API checks it.
export const marginPercentSchema = z.string().regex(/^[0-9]{1,3}(\.[0-9]{1,6})?$/).refine(value => {
  const [whole = '', fraction = ''] = value.split('.')
  return Number(whole) < 100 || (Number(whole) === 100 && !/[1-9]/.test(fraction))
})
export const errorJudgeSchema = z.object({ mode: z.enum(errorJudgeModes), epsilon: z.enum(errorJudgeEpsilons), marginPercent: marginPercentSchema.optional() })
export type ErrorJudge = z.infer<typeof errorJudgeSchema>
export const generatorsSchema = z.object({ input: generatorSchema, output: generatorSchema, validation: generatorSchema.default({ runtime: 'cpp23-gcc', source: '' }) })
export type Generators = z.infer<typeof generatorsSchema>
export const emptyGenerators = (): Generators => ({ input: { runtime: 'cpp23-gcc', source: '' }, output: { runtime: 'cpp23-gcc', source: '' }, validation: { runtime: 'cpp23-gcc', source: '' } })

// Partial scoring (ADR 0025): groups of cases with points, scored by "groups" (all AC) or by the aggregator's code.
// Cases name their groups, so a group's name is how membership refers to it.
export const maxScoringGroups = 100
export const maxFullScore = 1_000_000
export const scoringGroupSchema = z.object({ name: z.string(), points: z.number().int() })
export type ScoringGroup = z.infer<typeof scoringGroupSchema>
export const scoringSchema = z.object({
  mode: z.enum(['groups', 'aggregate']),
  groups: z.array(scoringGroupSchema).nullish().transform(groups => groups ?? []),
  aggregator: generatorSchema.nullish().transform(code => code ?? undefined),
})
export type Scoring = z.infer<typeof scoringSchema>

export const problemDraftSchema = z.object({
  difficulty: difficultySchema,
  checker: judgeCodeSchema.nullable().default(null),
  interactor: judgeCodeSchema.nullable().default(null),
  runTwice: judgeCodeSchema.nullable().default(null),
  errorJudge: errorJudgeSchema.nullable().default(null),
  generators: generatorsSchema.default(emptyGenerators),
  scoring: scoringSchema.nullable().default(null),
  title: z.string().max(120),
  markdown: z.string().max(100_000),
  editorial: z.string().max(100_000).default(''),
  tags: z.array(z.string()).default([]),
  timeLimitMs: z.string().max(10),
  memoryLimitMb: z.string().max(10),
  testCases: z.array(z.object({
    name: z.string().optional(), isSample: z.boolean().optional(), input: z.string().default(''), output: z.string().default(''),
    inputFile: testFileSchema.optional(), outputFile: testFileSchema.optional(),
    groups: z.array(z.string()).nullish().transform(groups => groups ?? []),
  })).default([]),
})
export type ProblemDraft = z.infer<typeof problemDraftSchema>

