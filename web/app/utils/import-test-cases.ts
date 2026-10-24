import { testFileLimit, testSetLimit } from '~~/shared/types/problem-draft'
import { testCaseError, type TestCase } from './problem-draft'

type DataKey = 'input' | 'output'
export function mergeTestCases(existing: TestCase[], imported: TestCase[], fields: DataKey[] = ['input', 'output']): TestCase[] {
  const replacements = new Map(imported.map(item => [item.name?.trim(), item]))
  const merged = existing.map(item => {
    const replacement = replacements.get(item.name?.trim())
    replacements.delete(item.name?.trim())
    if (!replacement) return item
    const updated = { ...item }
    for (const key of fields) {
      updated[key] = replacement[key]
      delete updated[`${key}File`]
      delete updated[`_${key}Dirty`]
    }
    return updated
  })
  return [...merged, ...replacements.values()]
}

export async function importTestCaseFiles(files: File[], key: DataKey, existing: TestCase[]): Promise<TestCase[]> {
  const names = new Set<string>()
  const encoder = new TextEncoder()
  for (const file of files) {
    if (!/^.+\.txt$/.test(file.name)) throw new Error('.txt ファイルを選択してください。')
    const name = file.name.trim()
    if (names.has(name)) throw new Error(`「${file.name}」が重複しています。`)
    if (file.size > testFileLimit) throw new Error(`「${file.name}」は16 MiB以内にしてください。`)
    names.add(name)
  }
  const total = existing.reduce((sum, item) => sum + (['input', 'output'] as const).reduce((size, field) => {
    if (field === key && names.has(item.name?.trim() ?? '')) return size
    return size + (item[`${field}File`] && !item[`_${field}Dirty`] ? item[`${field}File`]!.size : encoder.encode(item[field]).length)
  }, 0), 0) + files.reduce((sum, file) => sum + file.size, 0)
  if (total > testSetLimit) throw new Error('テストケース全体を512 MiB以内にしてください。')
  const imported = files.map(file => ({ name: file.name, input: '', output: '' }))
  const error = testCaseError(imported) || testCaseError(mergeTestCases(existing, imported, [key]))
  if (error) throw new Error(error)
  for (const [index, file] of files.entries()) {
    const bytes = await file.arrayBuffer()
    try {
      // Files written on Windows are CRLF, and Python's input() on Linux keeps the CR.
      imported[index]![key] = new TextDecoder('utf-8', { fatal: true }).decode(bytes).replace(/\r\n?/g, '\n')
    } catch {
      throw new Error(`「${file.name}」はUTF-8のテキストファイルにしてください。`)
    }
  }
  const contentError = testCaseError(mergeTestCases(existing, imported, [key]))
  if (contentError) throw new Error(contentError)
  return imported
}
