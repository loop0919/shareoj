import { expect, test } from '@playwright/test'
import { createIconLoader, type UserIcon } from '../app/utils/user-icons'

// A loader whose requests are answered by hand and whose scheduled send runs when the test says so.
function setup() {
  const requests: { handles: string[], resolve: (items: UserIcon[]) => void, reject: (error: unknown) => void }[] = []
  let scheduled: (() => void) | undefined
  let schedules = 0
  const loader = createIconLoader(
    handles => new Promise<UserIcon[]>((resolve, reject) => { requests.push({ handles, resolve, reject }) }),
    run => { schedules++; scheduled = run },
  )
  // Sends what has been asked for, as the page does after it has rendered its names.
  const send = () => { const run = scheduled; scheduled = undefined; run?.() }
  const settled = () => new Promise(resolve => setTimeout(resolve, 0))
  return { loader, requests, send, settled, schedules: () => schedules }
}

test('the names of one page go out as one request, each handle once', async () => {
  const { loader, requests, send, settled, schedules } = setup()
  for (const handle of ['alice', 'bob', 'alice', 'carol']) loader.want(handle)
  expect(requests).toHaveLength(0)
  expect(schedules()).toBe(1)
  send()
  expect(requests.map(request => request.handles)).toEqual([['alice', 'bob', 'carol']])
  requests[0]!.resolve([{ handle: 'alice', avatar: 'data:image/png;base64,AAAA' }, { handle: 'bob', avatar: '' }])
  await settled()
  expect(loader.icons.get('alice')).toBe('data:image/png;base64,AAAA')
  expect(loader.icons.get('bob')).toBe('')
  // A handle the answer leaves out has no profile; it is known to have no icon.
  expect(loader.icons.get('carol')).toBe('')
})

test('a handle that is known or on its way is not requested again', async () => {
  const { loader, requests, send, settled, schedules } = setup()
  loader.want('alice')
  send()
  loader.want('alice')
  loader.want('bob')
  send()
  expect(requests.map(request => request.handles)).toEqual([['alice'], ['bob']])
  requests[0]!.resolve([{ handle: 'alice', avatar: '' }])
  requests[1]!.resolve([])
  await settled()
  const before = schedules()
  loader.want('alice')
  loader.want('bob')
  expect(schedules()).toBe(before)
  send()
  expect(requests).toHaveLength(2)
})

test('more names than one request may carry are split', async () => {
  const { loader, requests, send } = setup()
  const handles = Array.from({ length: 45 }, (_, index) => `user_${index}`)
  for (const handle of handles) loader.want(handle)
  send()
  expect(requests.map(request => request.handles.length)).toEqual([20, 20, 5])
  expect(requests.flatMap(request => request.handles)).toEqual(handles)
})

test('a failed request leaves its handles unknown, and the next page that shows them asks again', async () => {
  const { loader, requests, send, settled } = setup()
  const handles = Array.from({ length: 21 }, (_, index) => `user_${index}`)
  for (const handle of handles) loader.want(handle)
  send()
  requests[0]!.reject(new Error('502'))
  requests[1]!.resolve([{ handle: 'user_20', avatar: 'data:image/png;base64,BBBB' }])
  await settled()
  expect(loader.icons.has('user_0')).toBe(false)
  expect(loader.icons.get('user_20')).toBe('data:image/png;base64,BBBB')
  loader.want('user_0')
  loader.want('user_20')
  send()
  expect(requests[2]!.handles).toEqual(['user_0'])
})
