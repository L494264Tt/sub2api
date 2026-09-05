import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { buildConversationTimeline } from '../conversationTimeline'
import ConversationTurnDetail from '../components/ConversationTurnDetail.vue'
import ConversationReadingView from '../components/ConversationReadingView.vue'
import { dialogueText, isAuxiliaryTurn, readingResponse } from '../conversationReading'
import type { ConversationMessage, ConversationTurn } from '../types'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ locale: { value: 'en' }, t: (key: string) => key }) }))
const message = (role: string, content: string, position: number): ConversationMessage => ({ role, content, position, kind: 'text', truncated: false })
const turn = (id: number, messages: ConversationMessage[], response: string): ConversationTurn => ({
  id, captured_at: `2026-09-05T00:0${id}:00Z`, request_id: `r${id}`, request_kind: 'conversation',
  request_details: { version: 1, messages, context: [message('system', 'hidden policy', -1)], current_message_position: messages.at(-1)?.position ?? -1, omitted_messages: 0, omitted_context: 0, context_truncated: false },
  model_response: response, categories: [], request_transcript: 'saved text', response_truncated: false,
} as ConversationTurn)

describe('Conversation timeline', () => {
  it('recognizes embedded action approvals in an existing conversation', () => {
    const approval = turn(1, [message('user', 'The following is the Codex agent history added since your last approval assessment.', 0)], '**Checking**{"outcome":"allow"}')
    approval.request_details!.context = [message('system', 'You are judging one planned coding-agent action.\npolicy', -1)]
    expect(isAuxiliaryTurn(approval)).toBe(true)
    expect(buildConversationTimeline([approval])).toEqual([])
  })
  it('does not replay old user messages when compaction omits assistant history', () => {
    const first = turn(1, [message('user', 'task', 0), message('assistant', 'progress', 1), message('user', 'execute', 2), message('assistant', 'more progress', 3)], '')
    const compressed = turn(2, [message('user', 'task', 0), message('user', 'execute', 1), message('user', 'execute', 2)], 'latest')
    expect(buildConversationTimeline([first, compressed])[1].messages.map(item => item.content)).toEqual(['execute'])
  })
  it('does not repeat unchanged input history during tool-only continuations', () => {
    const messages = [message('user', 'task', 0), message('assistant', 'working', 1)]
    const turns = [turn(1, messages, '**Reading****Inspecting**'), turn(2, messages, ''), turn(3, messages, ''), turn(4, [...messages, message('assistant', 'result', 2)], '')]
    const timeline = buildConversationTimeline(turns)
    expect(timeline.map(entry => entry.messages.map(item => item.content))).toEqual([['task', 'working'], ['result']])
  })
  it('removes only complete anchored client envelopes from the reading view', () => {
    expect(dialogueText('# AGENTS.md instructions for /repo\n<INSTRUCTIONS>policy</INSTRUCTIONS>\n<environment_context>workspace</environment_context>\nactual question')).toBe('actual question')
    expect(dialogueText('<skill><name>helper</name>policy</skill>')).toBe('')
    expect(dialogueText('Explain <skill>helper</skill>')).toBe('Explain <skill>helper</skill>')
    expect(dialogueText('<skill>unfinished')).toBe('<skill>unfinished')
  })
  it('recognizes previously stored heartbeat records without editing them', () => {
    const heartbeat = turn(1, [message('user', '[Sat 2026-09-05 07:21 UTC] [OpenClaw heartbeat poll]', 0)], 'HEARTBEAT_OK')
    expect(isAuxiliaryTurn(heartbeat)).toBe(true)
    expect(heartbeat.request_kind).toBe('conversation')
  })
  it('groups progress and keeps only the most recent reply expanded by default', () => {
    const first = turn(1, [message('user', '# AGENTS.md instructions for /repo\n<INSTRUCTIONS>noise</INSTRUCTIONS>', 0), message('user', 'Prepare the workbook', 1), message('assistant', 'Checking source data', 2)], 'Workbook is ready')
    const view = mount(ConversationReadingView, { props: { turns: [first] } })
    expect(view.text()).not.toContain('noise')
    expect(view.text()).not.toContain('request-')
    expect(view.text()).toContain('Prepare the workbook')
    expect(view.text()).toContain('Workbook is ready')
    expect(view.get('[data-test="progress-records"]').attributes('open')).toBeUndefined()
    view.unmount()
  })
  it('folds recognizable old reasoning prefixes only in the reading view', () => {
    const item = turn(1, [], '**Reading****Inspecting**Actual result')
    item.request_details!.response_context = [{ ...message('assistant', 'delta', 1), kind: 'tool_use_delta' }]
    expect(readingResponse(item)).toBe('Actual result')
    expect(item.model_response).toBe('**Reading****Inspecting**Actual result')
    expect(readingResponse({ ...item, model_response: '**Important**\nActual result' })).toBe('**Important**\nActual result')
  })
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
