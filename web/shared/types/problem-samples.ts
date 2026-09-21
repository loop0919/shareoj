import { z } from 'zod'

const sampleFileSchema = z.object({
  url: z.string().url(),
  size: z.number().int().positive(),
  sha256: z.string().regex(/^[a-f0-9]{64}$/),
})

export const problemSamplesSchema = z.object({
  items: z.array(z.object({
    name: z.string(),
    input: z.string(),
    output: z.string(),
    inputFile: sampleFileSchema.optional(),
    outputFile: sampleFileSchema.optional(),
  })),
})
