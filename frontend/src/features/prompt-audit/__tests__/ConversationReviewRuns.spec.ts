import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import ConversationReviewRuns from '../components/ConversationReviewRuns.vue'
import ConversationWorkspace from '../components/ConversationWorkspace.vue'

const api = vi.hoisted(() => ({ listConversationReviewRuns: vi.fn(), runConversationReview: vi.fn(), listConversations: vi.fn(), getConversation: vi.fn() }))
const app = vi.hoisted(() => ({ showError: vi.fn(), showSuccess: vi.fn() }))
vi.mock('../api', () => ({ default: api }))
vi.mock('@/stores/app', () => ({ useAppStore: () => app }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ locale: { value: 'en' }, t: (key: string) => key }) }))
const PaginationStub = defineComponent({ props: ['total', 'page', 'pageSize'], emits: ['update:page', 'update:pageSize'], template: '<div data-test="pagination" />' })
let wrapper: VueWrapper | undefined

function runs(page = 1, pageSize = 20, status = 'queued') {
  return { items: [{ id: page, created_at: '2026-09-05T00:00:00Z', trigger_type: 'manual', status, processed_count: 0, flagged_count: 0, failed_count: 0, last_error_code: '', last_error_message: '' }], total: 65, page, page_size: pageSize, pages: Math.ceil(65 / pageSize) }
}
async function start() {
  wrapper = mount(ConversationReviewRuns, { props: { enabled: true }, global: { stubs: { Pagination: PaginationStub } } })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  api.listConversationReviewRuns.mockImplementation(async (page: number, size: number) => runs(page, size))
  api.runConversationReview.mockResolvedValue({ id: 10 })
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('Conversation review runs', () => {
  it('loads older pages and resets to page one when page size changes', async () => {
    const view = await start()
    view.findComponent(PaginationStub).vm.$emit('update:page', 2)
    await flushPromises()
    expect(api.listConversationReviewRuns).toHaveBeenLastCalledWith(2, 20)
    expect(view.findComponent(PaginationStub).props('page')).toBe(2)
    view.findComponent(PaginationStub).vm.$emit('update:pageSize', 50)
    await flushPromises()
    expect(api.listConversationReviewRuns).toHaveBeenLastCalledWith(1, 50)
  })

  it('refreshes the current page and stops polling on unmount', async () => {
    const view = await start()
    view.findComponent(PaginationStub).vm.$emit('update:page', 2)
    await flushPromises()
    api.listConversationReviewRuns.mockResolvedValue(runs(2, 20, 'completed'))
    await vi.advanceTimersByTimeAsync(5000)
    await flushPromises()
    expect(api.listConversationReviewRuns).toHaveBeenLastCalledWith(2, 20)
    expect(view.text()).toContain('admin.promptAudit.reviewRuns.statuses.completed')
    view.unmount()
    wrapper = undefined
    const count = api.listConversationReviewRuns.mock.calls.length
    await vi.advanceTimersByTimeAsync(10000)
    expect(api.listConversationReviewRuns).toHaveBeenCalledTimes(count)
  })

  it('keeps the current page when loading another page fails', async () => {
    const view = await start()
    api.listConversationReviewRuns.mockRejectedValueOnce(new Error('offline'))
    view.findComponent(PaginationStub).vm.$emit('update:page', 2)
    await flushPromises()
    expect(view.findComponent(PaginationStub).props('page')).toBe(1)
    expect(view.get('[role="alert"]').text()).toContain('loadReviewRuns')
  })

  it('prevents duplicate submissions while queueing', async () => {
    const view = await start()
    let resolve!: (value: unknown) => void
    api.runConversationReview.mockImplementation(() => new Promise((done) => { resolve = done }))
    await view.get('[data-test="run-review"]').trigger('click')
    await view.get('[data-test="run-review"]').trigger('click')
    expect(api.runConversationReview).toHaveBeenCalledTimes(1)
    expect(view.get('[data-test="run-review"]').attributes('disabled')).toBeDefined()
    resolve({ id: 10 })
    await flushPromises()
    expect(api.listConversationReviewRuns).toHaveBeenLastCalledWith(1, 20)
  })

  it('does not poll hidden pages', async () => {
    await start()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    await vi.advanceTimersByTimeAsync(5000)
    expect(api.listConversationReviewRuns).toHaveBeenCalledTimes(1)
  })
})

it('displays an explicit warning for a truncated archived turn', async () => {
  const session = { id: 1, user_id: 1, group_id: 1, username: 'tester', last_turn_at: '2026-09-05T00:00:00Z', turns: [{ id: 1, captured_at: '2026-09-05T00:00:00Z', request_truncated: false, response_truncated: true, request_transcript: '[user]\nhello', model_response: 'partial', categories: [] }] }
  api.listConversations.mockResolvedValue({ items: [session], total: 1, page: 1, page_size: 20, pages: 1 })
  api.getConversation.mockResolvedValue(session)
  wrapper = mount(ConversationWorkspace)
  await flushPromises()
  const open = wrapper.findAll('button').find((button) => button.text() === 'common.view')!
  await open.trigger('click')
  await flushPromises()
  expect(wrapper.get('[role="status"]').text()).toBe('admin.promptAudit.conversations.readingGaps')
  await wrapper.findAll('[role="tab"]').find(tab => tab.text() === 'admin.promptAudit.conversations.fullConversation')!.trigger('click')
  expect(wrapper.get('[role="status"]').text()).toBe('admin.promptAudit.conversations.truncated')
})
