import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import UsageArchiveView from '../UsageArchiveView.vue'

const api = vi.hoisted(() => ({
  listSessions: vi.fn(),
  getStats: vi.fn(),
  getSession: vi.fn(),
  deleteSessionContent: vi.fn(),
}))

vi.mock('../api', () => ({ default: api }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: { value: 'en-US' } }) }))

describe('UsageArchiveView', () => {
  afterEach(() => vi.clearAllMocks())

  it('loads session metadata and archive statistics together', async () => {
    api.listSessions.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    api.getStats.mockResolvedValue({ sessions: 3, turns: 7, messages: 14, content_bytes: 1024, user_messages: 5, prompt_messages: 4, assistant_messages: 5 })

    mount(UsageArchiveView)
    await flushPromises()

    expect(api.listSessions).toHaveBeenCalled()
    expect(api.getStats).toHaveBeenCalledTimes(1)
  })
})
