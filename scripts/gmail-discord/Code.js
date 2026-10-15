// Notifies a Discord webhook of new mail in the Gmail inbox. Setup: docs/gmail-discord.md
// The gmail.metadata scope cannot read bodies. Subjects and senders are sent only when INCLUDE_DETAILS is true,
// because they may carry an inquirer's personal data and Discord is outside the privacy policy's storage.
const INCLUDE_DETAILS = false
const STATE_KEY = 'seenMessageIds'
// More new mail than this between two runs is reported on the next run at the latest.
const INBOX_WINDOW = 50
const SENDER_NAME = 'ShareOJ メール'

function setup() {
  for (const trigger of ScriptApp.getProjectTriggers()) {
    if (trigger.getHandlerFunction() === 'notifyNewMail') ScriptApp.deleteTrigger(trigger)
  }
  ScriptApp.newTrigger('notifyNewMail').timeBased().everyMinutes(5).create()
  notifyNewMail()
}

function testWebhook() {
  post({ content: `ShareOJのメール通知を設定しました。\n${inboxUrl()}` })
}

function notifyNewMail() {
  const lock = LockService.getScriptLock()
  if (!lock.tryLock(0)) return
  try {
    const properties = PropertiesService.getScriptProperties()
    const inbox = (Gmail.Users.Messages.list('me', { labelIds: ['INBOX'], maxResults: INBOX_WINDOW }).messages || []).map(m => m.id)
    const saved = properties.getProperty(STATE_KEY)
    // The first run marks the current inbox as seen instead of reporting old mail.
    if (saved === null) {
      properties.setProperty(STATE_KEY, JSON.stringify(inbox))
      return
    }
    const seen = new Set(JSON.parse(saved))
    const fresh = inbox.filter(id => !seen.has(id)).reverse()
    try {
      if (INCLUDE_DETAILS) {
        for (const id of fresh) {
          post(detail(id))
          seen.add(id)
        }
      } else if (fresh.length > 0) {
        post({ content: `新しいメールが${fresh.length}件届きました。\n${inboxUrl()}` })
        fresh.forEach(id => seen.add(id))
      }
    } finally {
      // Unsent mail stays unseen and is retried by the next run.
      properties.setProperty(STATE_KEY, JSON.stringify(inbox.filter(id => seen.has(id))))
    }
  } finally {
    lock.releaseLock()
  }
}

function detail(id) {
  const message = Gmail.Users.Messages.get('me', id, { format: 'metadata', metadataHeaders: ['Subject', 'From'] })
  const header = name => ((message.payload && message.payload.headers) || []).find(h => h.name.toLowerCase() === name.toLowerCase())?.value || ''
  return {
    embeds: [{
      title: truncate(header('Subject') || '（件名なし）', 256),
      url: `${inboxUrl()}/${message.threadId}`,
      fields: [{ name: '差出人', value: truncate(header('From') || '（不明）', 1024) }],
      timestamp: new Date(Number(message.internalDate)).toISOString(),
    }],
  }
}

let mailbox = ''
// The account in the URL opens this mailbox even when another Google account is the browser's default.
function inboxUrl() {
  mailbox = mailbox || Gmail.Users.getProfile('me').emailAddress
  return `https://mail.google.com/mail/?authuser=${encodeURIComponent(mailbox)}#inbox`
}

function post(message) {
  const url = PropertiesService.getScriptProperties().getProperty('DISCORD_WEBHOOK_URL') || ''
  if (!/^https:\/\/(?:discord|discordapp)\.com\/api\/webhooks\//.test(url)) {
    throw new Error('スクリプト プロパティのDISCORD_WEBHOOK_URLに、DiscordのウェブフックURLを設定してください。')
  }
  // Mail text must never ping @everyone or a role.
  const payload = JSON.stringify({ username: SENDER_NAME, allowed_mentions: { parse: [] }, ...message })
  for (let attempt = 0; ; attempt++) {
    const response = UrlFetchApp.fetch(url, { method: 'post', contentType: 'application/json', payload, muteHttpExceptions: true })
    const status = response.getResponseCode()
    if (status < 300) return
    if (status !== 429 || attempt > 0) throw new Error(`Discordへの送信に失敗しました（HTTP ${status}）`)
    Utilities.sleep(Math.ceil(retryAfter(response) * 1000))
  }
}

function retryAfter(response) {
  try { return Math.min(Number(JSON.parse(response.getContentText()).retry_after) || 1, 30) }
  catch (error) { return 1 }
}

function truncate(text, length) {
  return text.length > length ? `${text.slice(0, length - 1)}…` : text
}
