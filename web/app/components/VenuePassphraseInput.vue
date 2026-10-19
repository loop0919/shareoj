<script setup lang="ts">
import { venuePassphraseMessage, venuePassphrasePattern, venuePassphraseValid } from '~/utils/venue-passphrase'
// A venue passphrase field for setters and participants (ADR 0024). The pattern guides typing; the browser then
// reports the rule in words, and an empty field is left to required. Case matters, so nothing may capitalise or correct
// what is typed.
const value = defineModel<string>({ required: true })
const input = ref<HTMLInputElement>()
function check() { input.value?.setCustomValidity(value.value && !venuePassphraseValid(value.value) ? venuePassphraseMessage : '') }
watch(value, check, { flush: 'post' })
onMounted(check)
</script>

<template>
  <input ref="input" v-model="value" :pattern="venuePassphrasePattern" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false">
</template>
