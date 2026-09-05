import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { buildConversationTimeline } from '../conversationTimeline'
import ConversationTurnDetail from '../components/ConversationTurnDetail.vue'
import type { ConversationMessage, ConversationTurn } from '../types'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ locale: { value: 'en' }, t: (key: string) => key }) }))
const message = (role: string, content: string, position: number): ConversationMessage => ({ role, content, position, kind: 'text', truncated: false })
const turn = (id: number, messages: ConversationMessage[], response: string): ConversationTurn => ({
  id, captured_at: `2026-09-05T00:0${id}:00Z`, request_id: `r${id}`, request_kind: 'conversation',
  request_details: { version: 1, messages, context: [message('system', 'hidden policy', -1)], current_message_position: messages.at(-1)?.position ?? -1, omitted_messages: 0, omitted_context: 0, context_truncated: false },
  model_response: response, categories: [], request_transcript: 'saved text', response_truncated: false,
} as ConversationTurn)

describe('Conversation timeline', () => {
  it('displays repeated history only once while retaining the next exchange', () => {
    const first = turn(1, [message('user', 'one', 0)], 'answer one')
    const second = turn(2, [message('user', 'one', 0), message('assistant', 'answer one', 1), message('user', 'two', 2)], 'answer two')
    const timeline = buildConversationTimeline([second, first])
    expect(timeline.map(entry => entry.messages.map(item => item.content))).toEqual([['one'], ['two']])
    expect(timeline.map(entry => entry.turn.model_response)).toEqual(['answer one', 'answer two'])
  })
  it('does not discard the same text under another role or hide new standalone history', () => {
    const first = turn(1, [message('user', 'one', 0)], 'answer')
    const second = turn(2, [message('user', 'answer', 0)], 'next')
    expect(buildConversationTimeline([first, second])[1].messages).toHaveLength(1)
  })
  it('keeps auxiliary requests outside the default dialogue timeline', () => {
    expect(buildConversationTimeline([{ ...turn(1, [], 'no'), request_kind: 'auxiliary' }])).toEqual([])
  })
  it('renders user dialogue by default and leaves system and saved-request details collapsed', () => {
    const view = mount(ConversationTurnDetail, { props: { turn: turn(1, [message('user', 'my question', 0)], 'my answer') } })
    expect(view.get('[data-test="current-messages"]').text()).toContain('my question')
    expect(view.get('[data-test="current-messages"]').text()).not.toContain('hidden policy')
    expect(view.get('[data-test="model-response"]').text()).toContain('my answer')
    expect(view.get('[data-test="context"]').attributes('open')).toBeUndefined()
    expect(view.get('[data-test="saved-request"]').attributes('open')).toBeUndefined()
    view.unmount()
  })
  it('does not invent message roles for legacy flattened archives', () => {
    const legacy = turn(1, [], 'answer'); delete legacy.request_details
    const view = mount(ConversationTurnDetail, { props: { turn: legacy } })
    expect(view.find('[data-test="legacy-note"]').exists()).toBe(true)
    expect(view.find('[data-test="current-messages"]').exists()).toBe(false)
    view.unmount()
  })
})
