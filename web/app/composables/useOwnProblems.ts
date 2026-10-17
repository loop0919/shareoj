import { accountListSchema, type AccountSummary } from '~~/shared/types/account-problems'
import { accountError } from '~/utils/account-problems'

// Every page of the signed-in author's problems, for pages that choose one of them.
export function useOwnProblems() {
  const { refreshAccount } = useAccount()
  const problems = ref<AccountSummary[]>([])
  const signedIn = ref(false)
  const loading = ref(true)
  const message = ref('')
  async function load() {
    loading.value = true
    message.value = ''
    try {
      signedIn.value = !!await refreshAccount()
      const items: AccountSummary[] = []
      let cursor = ''
      while (signedIn.value) {
        const page = accountListSchema.parse(await $fetch('/api/my/problems', { query: { cursor } }))
        items.push(...page.items)
        cursor = page.nextCursor
        if (!cursor) break
      }
      problems.value = items
    } catch (error) { message.value = accountError(error) }
    finally { loading.value = false }
  }
  return { problems, signedIn, loading, message, load }
}
