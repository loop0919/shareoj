<script setup lang="ts">
import { venuePassphraseBlank, venuePassphraseMessage, venuePassphraseValid } from '~/utils/venue-passphrase'
// A venue passphrase field for setters and participants (ADR 0024). It checks the rule after the server's folding and
// trimming, so the browser stops exactly the values the server refuses, and reports the rule in words. Spaces alone are
// empty: an optional field then joins open, and a required one names the rule. Case matters, so nothing may capitalise
// or correct what is typed.
const props = defineProps<{ required?: boolean }>()
const value = defineModel<string>({ required: true })
const input = ref<HTMLInputElement>()
function check() {
  const blank = venuePassphraseBlank(value.value)
  // An empty required field is left to required.
  const invalid = blank ? props.required && value.value !== '' : !venuePassphraseValid(value.value)
  input.value?.setCustomValidity(invalid ? venuePassphraseMessage : '')
}
watch(value, check, { flush: 'post' })
onMounted(check)
</script>

<template>
  <input ref="input" v-model="value" :required="required" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false">
</template>
