import { z } from 'zod'
// The API's limits: folders per person, levels, characters of a name after trimming, and items one move sends.
export const maxFolders = 100, maxFolderDepth = 3, maxFolderName = 40, maxFolderItems = 100
export const folderSchema = z.object({ id: z.string().uuid(), parentId: z.string().uuid().nullable(), name: z.string() })
// Own and tested problems are both a problem; a placement belongs to the person who made it.
export const folderRefSchema = z.object({ kind: z.enum(['problem', 'post']), id: z.string().uuid() })
// Only placements in a folder are listed; whatever has none is unsorted.
export const folderTreeSchema = z.object({ folders: z.array(folderSchema), items: z.array(folderRefSchema.extend({ folderId: z.string().uuid() })) })
export type Folder = z.infer<typeof folderSchema>
export type FolderRef = z.infer<typeof folderRefSchema>
