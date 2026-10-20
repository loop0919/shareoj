<script setup lang="ts">
import { contestDate, type Contest, type Entrant, type Venue } from '~~/shared/types/contest'
import { contestTeamError, errorCode } from '~/utils/contest-team'
import { venuePassphraseRule } from '~/utils/venue-passphrase'
// Setters replace the venue passphrase until the end, and move an open participant or team onsite and rename teams
// until the results are final; nothing moves back to open. The editor locks at the start, so these live on the contest
// page (ADR 0024).
const props = defineProps<{ contest: Contest }>()
const emit = defineEmits<{ changed: [] }>()
const base = computed(() => `/api/my/contests/${props.contest.id}`)
const division = computed(() => props.contest.onsiteDivision === true)
const teamMode = computed(() => props.contest.teamMode === true)
const ended = computed(() => props.contest.status === 'ended')
const final = computed(() => props.contest.resultsFinal ?? ended.value)
const venue = ref<Venue | null>(null)
const entrants = ref<Entrant[] | null>(null)
const loading = ref(false)
const loadError = ref('')
// The row or the passphrase being changed; one change runs at a time.
const busy = ref('')
const message = ref('')
const failed = ref(false)
const rotating = ref(false)
const newPassphrase = ref('')
const renaming = ref('')
const newName = ref('')
// Each form replaces the button that opened it, and closing it brings the button back; focus follows, rather than
// falling to the page.
const root = ref<HTMLDetailsElement>()
async function focus(selector: string) {
  await nextTick()
  root.value?.querySelector<HTMLElement>(selector)?.focus()
}
const key = (row: Entrant) => row.team?.id ?? row.handle
const label = (row: Entrant) => row.team?.name ?? row.handle
// Team contests list teams and those who joined alone in one table.
const teams = computed(() => entrants.value?.filter(row => row.team).length ?? 0)
function report(text: string, error = false) { message.value = text; failed.value = error }
// The passphrase is shown only once a setter opens the panel, so a page projected at the venue does not show it.
let opened = false
function toggle(event: Event) {
  if (!(event.target as HTMLDetailsElement).open || opened) return
  opened = true
  void load()
}
async function load() {
  loading.value = true; loadError.value = ''
  try {
    const [current, list] = await Promise.all([
      division.value ? $fetch<Venue>(`${base.value}/onsite`) : Promise.resolve(null),
      $fetch<{ items: Entrant[] }>(`${base.value}/participants`),
    ])
    venue.value = current; entrants.value = list.items
  } catch { loadError.value = '参加者の一覧を取得できませんでした。作成者のままか確かめて、再読み込みしてください。' }
  finally { loading.value = false }
}
function failure(error: unknown, after: string) {
  if (errorCode(error) === 'contest_conflict') return after
  return contestTeamError(error, '変更できませんでした。一覧を更新してから、もう一度お試しください。')
}
function startRotate() { newPassphrase.value = ''; rotating.value = true; report(''); void focus('#setter-venue-new') }
function closeRotate() { rotating.value = false; void focus('#setter-venue-rotate') }
async function rotate() {
  if (busy.value) return
  busy.value = 'venue'; report('')
  try {
    venue.value = await $fetch<Venue>(`${base.value}/onsite`, { method: 'PUT', body: { passphrase: newPassphrase.value } })
    rotating.value = false
    report('会場の合言葉を差し替えました。前の合言葉は使えません。')
  } catch (error) { report(failure(error, '終了したため、会場の合言葉は差し替えられません。'), true) }
  finally { busy.value = '' }
  if (!rotating.value) void focus('#setter-venue-rotate')
}
// There is no way back to open, so the move asks first.
async function goOnsite(row: Entrant) {
  if (busy.value || !window.confirm(`${label(row)}をオンサイト参加にしますか？オープン参加には戻せません。`)) return
  busy.value = key(row); report('')
  const path = row.team ? `teams/${row.team.id}/onsite` : `participants/${encodeURIComponent(row.handle)}/onsite`
  try {
    const updated = await $fetch<Entrant>(`${base.value}/${path}`, { method: 'PUT', body: { onsite: true } })
    entrants.value = entrants.value!.map(item => key(item) === key(updated) ? updated : item)
    report(`${label(row)}をオンサイト参加にしました。`)
    emit('changed')
  } catch (error) { report(failure(error, '結果が確定したため、区分は変更できません。'), true) }
  finally { busy.value = '' }
  // The button goes with the change, so focus stays on the row's division.
  if (!failed.value) void focus(`[data-division="${key(row)}"]`)
}
function startRename(row: Entrant) { newName.value = row.team?.name ?? ''; renaming.value = key(row); report(''); void focus('.setter-rename input') }
function closeRename(row: Entrant) { renaming.value = ''; void focus(`[data-rename="${key(row)}"]`) }
async function rename(row: Entrant) {
  if (busy.value || !row.team) return
  busy.value = key(row); report('')
  try {
    const updated = await $fetch<Entrant>(`${base.value}/teams/${row.team.id}/name`, { method: 'PUT', body: { name: newName.value } })
    entrants.value = entrants.value!.map(item => key(item) === key(updated) ? updated : item)
    renaming.value = ''
    report(`チーム名を「${updated.team?.name}」に変更しました。`)
    emit('changed')
  } catch (error) { report(failure(error, '結果が確定したため、チーム名は変更できません。'), true) }
  finally { busy.value = '' }
  if (!renaming.value) void focus(`[data-rename="${key(row)}"]`)
}
</script>

<template>
  <details ref="root" class="setter-tools" @toggle="toggle">
    <summary><span>参加者の管理</span><span class="muted">作成者だけに表示</span></summary>
    <div class="setter-tools-body">
      <p class="muted">{{ division ? `会場の合言葉は終了まで差し替えられます。${teamMode ? 'オープン参加の人とチーム' : 'オープン参加の人'}は、結果が確定するまでオンサイト参加にできます。オープン参加には戻せません。` : 'チーム名は、結果が確定するまで変更できます。' }}</p>
      <p v-if="loadError" class="notice notice-error" role="alert">{{ loadError }}</p>
      <LoadingSpinner v-else-if="!entrants" label="参加者の一覧を読み込み中" />
      <template v-else>
        <section v-if="division" class="setter-venue" aria-labelledby="setter-venue-title">
          <h3 id="setter-venue-title">会場の合言葉</h3>
          <div v-if="!rotating" class="setter-row"><input id="setter-venue-current" :value="venue?.passphrase ?? ''" readonly aria-labelledby="setter-venue-title" @focus="($event.target as HTMLInputElement).select()"><button v-if="!ended" id="setter-venue-rotate" type="button" class="editor-button" :disabled="!!busy" @click="startRotate">差し替える</button></div>
          <form v-else class="setter-form" @submit.prevent="rotate">
            <label for="setter-venue-new">新しい会場の合言葉</label>
            <div class="setter-row"><VenuePassphraseInput id="setter-venue-new" v-model="newPassphrase" required aria-describedby="setter-venue-rule" /><button type="submit" class="editor-button" :disabled="!!busy" :aria-busy="busy === 'venue'">{{ busy === 'venue' ? '保存中…' : '保存' }}</button><button type="button" class="editor-button" :disabled="busy === 'venue'" @click="closeRotate">キャンセル</button></div>
            <p id="setter-venue-rule" class="muted">{{ venuePassphraseRule }}です。大文字と小文字は区別します。</p>
          </form>
          <p class="muted">会場の参加者に配ります。差し替えると前の合言葉は使えなくなりますが、すでにオンサイト参加の人はそのままです。</p>
        </section>
        <section class="setter-entrants" aria-labelledby="setter-entrants-title">
          <div class="setter-heading"><h3 id="setter-entrants-title">{{ teamMode ? 'チーム・個人' : '参加者' }} <span class="muted">{{ teamMode ? `チーム ${teams}・個人 ${entrants.length - teams}` : `${entrants.length} 人` }}</span></h3><RefreshButton label="参加者の一覧を更新" :busy="loading" :disabled="loading" @click="load" /></div>
          <p v-if="!entrants.length" class="muted">まだ誰も参加していません。</p>
          <div v-else class="content-table-scroll" role="region" aria-label="参加者の一覧" tabindex="0">
            <table class="content-table setter-table">
              <thead><tr><th scope="col">{{ teamMode ? 'チーム・個人' : '参加者' }}</th><th v-if="division" scope="col">区分</th><th v-if="division" scope="col">オンサイトになった時刻 (JST)</th><th scope="col">登録日時 (JST)</th></tr></thead>
              <tbody>
                <tr v-for="row in entrants" :key="key(row)">
                  <th scope="row">
                    <template v-if="row.team">
                      <form v-if="renaming === key(row)" class="setter-row setter-rename" @submit.prevent="rename(row)"><input v-model="newName" required autocomplete="off" :aria-label="`${row.team.name}の新しいチーム名`"><button type="submit" class="editor-button" :disabled="!!busy" :aria-busy="busy === key(row)">保存</button><button type="button" class="editor-button" :disabled="busy === key(row)" @click="closeRename(row)">キャンセル</button></form>
                      <span v-else class="setter-team"><strong>{{ row.team.name }}</strong><button v-if="!final" type="button" class="editor-button" :data-rename="key(row)" :disabled="!!busy" :aria-label="`${row.team.name}の名前を変更`" @click="startRename(row)">名前を変更</button></span>
                      <span class="setter-members"><template v-for="(member, index) in row.team.members" :key="member"><span v-if="index">、</span><UserLink :handle="member" /></template></span>
                    </template>
                    <template v-else><UserLink :handle="row.handle" /><span v-if="teamMode" class="setter-individual">個人参加</span></template>
                  </th>
                  <td v-if="division"><span class="setter-division" :data-division="key(row)" tabindex="-1" :data-onsite="row.onsite || undefined">{{ row.onsite ? 'オンサイト' : 'オープン' }}</span><button v-if="!row.onsite && !final" type="button" class="editor-button" :disabled="!!busy" :aria-busy="busy === key(row)" :aria-label="`${label(row)}をオンサイト参加にする`" @click="goOnsite(row)">{{ busy === key(row) ? '変更中…' : 'オンサイトにする' }}</button></td>
                  <td v-if="division" class="setter-time">{{ row.onsiteAt ? `${contestDate(row.onsiteAt)}（${row.onsiteVia === 'setter' ? '作成者' : '合言葉'}）` : '—' }}</td>
                  <td class="setter-time">{{ contestDate(row.createdAt) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
      </template>
      <p v-if="message" :class="failed ? 'notice notice-error' : 'setter-message muted'" :role="failed ? 'alert' : 'status'">{{ message }}</p>
    </div>
  </details>
</template>

<style scoped>
/* Hallmark · component: setter tools on the contest page · genre: modern-minimal · theme: existing ShareOJ tokens (Plain)
 * states: closed · loading · error · empty · renaming · moving onsite · locked after the results are final
 * Closed by default so a projected overview never shows the passphrase; inside, the same ruled tables as the page. */
.setter-tools { margin-block: 8px 32px; border: 1px solid var(--color-line); border-radius: 4px; }
.setter-tools summary { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 12px; min-height: 48px; padding: 8px 16px; font-weight: 600; cursor: pointer; }
.setter-tools summary .muted { font-size: .8125rem; font-weight: 400; }
.setter-tools summary:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 2px; }
.setter-tools[open] summary { border-bottom: 1px solid var(--color-line); }
.setter-tools-body { display: grid; grid-template-columns: minmax(0, 1fr); gap: 16px; padding: 16px; min-width: 0; }
.setter-tools-body p { margin: 0; font-size: .8125rem; }
.setter-tools h3 { margin: 0 0 8px; font-size: 1rem; }
.setter-tools h3 .muted { font-size: .8125rem; font-weight: 400; }
.setter-venue, .setter-form { display: grid; gap: 6px; max-width: 36rem; }
.setter-venue h3 { margin: 0; }
.setter-form label { font-size: .8125rem; font-weight: 600; }
.setter-row { display: flex; flex-wrap: wrap; gap: 8px; }
.setter-row input { flex: 1 1 12rem; min-width: 0; min-height: 44px; padding: 8px 10px; border: 1px solid var(--color-line); border-radius: 4px; font: inherit; }
.setter-row input:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 2px; }
#setter-venue-current { font-family: var(--font-code); }
.setter-tools .editor-button { min-height: 44px; padding: 8px 14px; }
.setter-tools .editor-button:disabled { opacity: .6; cursor: not-allowed; }
.setter-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.setter-heading h3 { margin: 0; }
.setter-table th[scope="row"] { min-width: 12rem; font-weight: 400; text-align: left; }
.setter-team { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 4px 8px; overflow-wrap: anywhere; }
.setter-members { display: block; margin-top: 4px; color: var(--color-muted); font-size: .8125rem; }
.setter-individual { margin-inline-start: 8px; color: var(--color-muted); font-size: .8125rem; }
.setter-table td:has(.setter-division) { white-space: nowrap; }
.setter-division { display: inline-block; margin-inline-end: 8px; padding: 2px 8px; border: 1px solid var(--color-line); border-radius: 4px; color: var(--color-muted); font-size: .8125rem; }
.setter-division[data-onsite] { border-color: var(--color-accent); color: var(--color-accent); background: var(--color-accent-soft); }
.setter-division:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 2px; }
.setter-time { white-space: nowrap; font-variant-numeric: tabular-nums; }
.setter-message { color: var(--color-muted); }
</style>
