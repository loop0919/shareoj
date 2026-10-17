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
export const generatorsSchema = z.object({ input: generatorSchema, output: generatorSchema, validation: generatorSchema.default({ runtime: 'cpp23-gcc', source: '' }) })
export type Generators = z.infer<typeof generatorsSchema>
export const emptyGenerators = (): Generators => ({ input: { runtime: 'cpp23-gcc', source: '' }, output: { runtime: 'cpp23-gcc', source: '' }, validation: { runtime: 'cpp23-gcc', source: '' } })

export const problemDraftSchema = z.object({
  difficulty: difficultySchema,
  checker: judgeCodeSchema.nullable().default(null),
  interactor: judgeCodeSchema.nullable().default(null),
  generators: generatorsSchema.default(emptyGenerators),
  title: z.string().max(120),
  markdown: z.string().max(100_000),
  editorial: z.string().max(100_000).default(''),
  tags: z.array(z.string()).default([]),
  timeLimitMs: z.string().max(10),
  memoryLimitMb: z.string().max(10),
  testCases: z.array(z.object({
    name: z.string().optional(), isSample: z.boolean().optional(), input: z.string().default(''), output: z.string().default(''),
    inputFile: testFileSchema.optional(), outputFile: testFileSchema.optional(),
  })).default([]),
})
export type ProblemDraft = z.infer<typeof problemDraftSchema>

