import { streamingCookie } from '../../shared/utils/streaming-mode'
import { cookieOptions } from '../utils/private-session'
import { streamingMode } from '../utils/streaming-mode'

// Every page the server renders in streaming mode sets the cookie again, so the mode lasts a year from the last visit and
// no cap on cookies written by script (seven days in Safari) ends it. It only renews: an absent cookie or another value is
// left alone, and API responses never carry it.
export default defineNitroPlugin(nitroApp => {
  nitroApp.hooks.hook('render:response', (_response, { event }) => {
    if (streamingMode(event)) setCookie(event, streamingCookie, '1', { ...cookieOptions(event), httpOnly: false, maxAge: 60 * 60 * 24 * 365 })
  })
})
