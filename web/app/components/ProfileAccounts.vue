<script setup lang="ts">
import { accountServices, type Profile } from '~~/shared/types/profile'
import { ratingColor } from '~~/shared/utils/rating'
const props = defineProps<{ accounts: Profile['accounts'] }>()
const links = computed(() => accountServices.filter(service => props.accounts[service.key]))
const { data: ratings } = useAsyncData(
  () => `account-ratings:${props.accounts.atcoder}:${props.accounts.codeforces}`,
  async () => {
    const entries = await Promise.all((['atcoder', 'codeforces'] as const).map(async service => {
      const handle = props.accounts[service]
      const result = handle
        ? await $fetch(`/api/ratings/${service}`, { query: { handle } }).catch(() => ({ rating: null, unavailable: true }))
        : { rating: null, unavailable: false }
      return [service, result] as const
    }))
    return Object.fromEntries(entries)
  }, { server: false },
)
const { data: yukicoder } = useAsyncData(
  () => `yukicoder-name:${props.accounts.yukicoder}`,
  () => props.accounts.yukicoder
    ? $fetch('/api/accounts/yukicoder', { query: { id: props.accounts.yukicoder } }).catch(() => ({ name: null }))
    : Promise.resolve({ name: null }),
  { server: false },
)
function color(service: typeof accountServices[number]['key']) {
  return service === 'atcoder' || service === 'codeforces' ? ratingColor(service, ratings.value?.[service]?.rating ?? null) : 'unrated'
}
</script>
<template>
  <ul v-if="links.length" class="profile-accounts" aria-label="外部アカウント">
    <li v-for="service in links" :key="service.key">
      <span>{{ service.label }}</span>
      <a :href="service.url + encodeURIComponent(accounts[service.key])" target="_blank" rel="noopener noreferrer" :class="[service.key, color(service.key)]">{{ service.key === 'yukicoder' ? (yukicoder?.name ?? accounts.yukicoder) : accounts[service.key] }}</a>
      <small v-if="service.key === 'atcoder' || service.key === 'codeforces'">
        {{ ratings?.[service.key]?.unavailable ? 'レーティング取得不可' : ratings?.[service.key] ? (ratings[service.key]?.rating ?? '未レート') : '取得中…' }}
      </small>
    </li>
  </ul>
</template>
<style scoped>
.profile-accounts { display: flex; flex-wrap: wrap; gap: 12px 28px; padding: 0; margin: 24px 0 28px; list-style: none; }
li { display: flex; align-items: baseline; flex-wrap: wrap; gap: 8px; min-width: 0; }
li > span, small { color: var(--color-muted); font-size: .85rem; }
a { font-weight: 700; overflow-wrap: anywhere; }
.unrated { color: var(--color-ink); }
.gray { color: #808080; } .brown { color: light-dark(#804000, #c58a50); } .green { color: #008000; }
.cyan { color: #00c0c0; } .blue { color: light-dark(#0000ff, #719cff); } .yellow { color: #c0c000; }
.orange { color: #ff8000; } .red, .legendary { color: #f00; } .violet { color: #a0a; }
.codeforces.cyan { color: #03a89e; } .codeforces.orange { color: #ff8c00; }
.legendary { display: inline-block; }
.legendary::first-letter { color: var(--color-ink); }
</style>
