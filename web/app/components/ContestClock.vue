<script setup lang="ts">
const props = defineProps<{ id: string }>()
const { data: contest } = useContest(() => props.id)
// The clock starts after mounting so the server's time never meets the browser's at hydration.
const now = ref<number>()
let timer: ReturnType<typeof setTimeout> | undefined
function tick() {
  now.value = Date.now()
  timer = setTimeout(tick, 1000 - now.value % 1000)
}
onMounted(tick)
// Clicking fades the clock to see what it covers; another click brings it back.
const faded = ref(false)
onBeforeUnmount(() => clearTimeout(timer))
const jst = new Intl.DateTimeFormat('ja-JP', { timeZone: 'Asia/Tokyo', year: 'numeric', month: '2-digit', day: '2-digit', weekday: 'short', hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23' })
// 2026/10/14（水） over 21:03:05.
function jstClock(ms: number) {
  const part = Object.fromEntries(jst.formatToParts(ms).map(({ type, value }) => [type, value]))
  return { label: `${part.year}/${part.month}/${part.day}（${part.weekday}）`, value: `${part.hour}:${part.minute}:${part.second}` }
}
function countdown(ms: number) {
  const total = Math.ceil(ms / 1000)
  const days = Math.floor(total / 86400)
  const time = `${Math.floor(total / 3600) % 24}:${String(Math.floor(total / 60) % 60).padStart(2, '0')}:${String(total % 60).padStart(2, '0')}`
  return days ? `${days}日 ${time}` : time
}
// The phase follows the browser clock, so it switches on time without waiting for the next poll.
const clock = computed(() => {
  if (!contest.value || now.value === undefined) return null
  const startsAt = new Date(contest.value.startsAt).getTime()
  const endsAt = new Date(contest.value.endsAt).getTime()
  if (now.value < startsAt) return { phase: 'scheduled', label: '開始まで', value: countdown(startsAt - now.value) }
  if (now.value < endsAt) return { phase: 'running', label: '終了まで', value: countdown(endsAt - now.value) }
  return { phase: 'ended', ...jstClock(now.value) }
})
</script>
<template>
  <button v-if="clock" type="button" class="contest-clock" :data-phase="clock.phase" :aria-pressed="faded" :title="faded ? 'クリックで元の濃さに戻す' : 'クリックで薄くする'" @click="faded = !faded">
    <span class="contest-clock-label">{{ clock.label }}</span>
    <span class="contest-clock-value">{{ clock.value }}</span>
  </button>
</template>
<style scoped>
.contest-clock { position: fixed; right: 16px; bottom: 16px; z-index: 50; display: grid; justify-items: end; padding: 8px 18px; border: 2px solid var(--color-accent); border-radius: 8px; background: var(--color-paper); box-shadow: 0 4px 12px #0002; color: var(--color-ink); font: inherit; line-height: 1.4; cursor: pointer; transition: opacity .15s; }
.contest-clock[aria-pressed="true"] { opacity: .1; }
.contest-clock-label { color: var(--color-muted); font-size: .8125rem; }
.contest-clock-value { font-size: 1.5rem; font-weight: 700; font-variant-numeric: tabular-nums; white-space: nowrap; }
.contest-clock[data-phase="running"] .contest-clock-value { color: var(--color-accent); }
</style>
