<script setup lang="ts">
import type { Contest } from '~~/shared/types/contest'
import { venuePassphraseRule } from '~/utils/venue-passphrase'
// An open participant, or a member of an open team, enters the venue passphrase until the end to turn onsite (ADR 0024).
const props = defineProps<{ contest: Contest }>()
const emit = defineEmits<{ joined: [contest: Contest] }>()
const { showToast } = useToast()
const { saving, error, send } = useContestJoin(() => props.contest.id)
const venue = ref('')
async function goOnsite() {
  if (saving.value) return
  const result = await send({ onsitePassphrase: venue.value })
  if (!result) return
  emit('joined', result)
  // Success is what the contest says now, so the page never reports onsite while the participant stays open.
  if (!result.onsite) { error.value = 'オンサイト参加に変わりませんでした。ページを再読み込みしてから、もう一度お試しください。'; return }
  venue.value = ''
  showToast('オンサイト参加に変わりました')
}
</script>

<template>
  <form class="participation-form" @submit.prevent="goOnsite">
    <div class="participation-field"><label for="venue-passphrase">会場の合言葉（オンサイト参加者のみ）</label><VenuePassphraseInput id="venue-passphrase" v-model="venue" required aria-describedby="venue-note" /></div>
    <button type="submit" class="editor-button" :disabled="saving" :aria-busy="saving">{{ saving ? '確認中…' : 'オンサイト参加にする' }}</button>
    <p id="venue-note" class="muted">{{ venuePassphraseRule }}です。会場で配られた合言葉を入れると、終了までオンサイト参加に変えられます。<template v-if="contest.team">メンバーの1人が入れると、チーム全体がオンサイト参加になります。</template>オンサイト参加は順位表に表示されます。</p>
    <p v-if="error" class="notice notice-error" role="alert">{{ error }}</p>
  </form>
</template>
