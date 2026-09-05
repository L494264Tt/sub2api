import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { buildOpenClawActivity } from '../openClawActivity'
import ConversationReadingView from '../components/ConversationReadingView.vue'
import type { ConversationTurn } from '../types'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ locale: { value: 'en' }, t: (key: string) => key }) }))

function heartbeat(): ConversationTurn {
  return {
    id: 1, captured_at: '2026-09-05T07:38:00Z', model_response: 'HEARTBEAT_OK', categories: [], request_kind: 'auxiliary',
    request_details: {
      version: 1, messages: [{ role: 'user', kind: 'text', content: '[Sat 2026-09-05 07:38 UTC] [OpenClaw heartbeat poll]', position: 1, truncated: false }],
      context: [
        { role: 'system', kind: 'text', content: 'You are a personal assistant running inside OpenClaw.', position: 0, truncated: false },
        { role: 'system', kind: 'tool_definitions', content: '[{"name":"delete_file"}]', position: 2, truncated: false },
        { role: 'assistant', kind: 'tool_use', content: '{"name":"session_status"}', position: 3, truncated: false },
      ], response_context: [], current_message_position: 1, omitted_messages: 0, omitted_context: 0, context_truncated: false,
    },
  } as ConversationTurn
}

describe('OpenClaw activity', () => {
  it('shows a heartbeat receipt instead of an empty conversation', () => {
    const wrapper = mount(ConversationReadingView, { props: { turns: [heartbeat()] } })
    expect(wrapper.get('[data-test="openclaw-activity"]').text()).toContain('admin.promptAudit.openClaw.heartbeat')
    expect(wrapper.text()).toContain('HEARTBEAT_OK')
    expect(wrapper.text()).toContain('admin.promptAudit.openClaw.noCalls')
    expect(wrapper.text()).not.toContain('admin.promptAudit.conversations.noReadableDialogue')
    wrapper.unmount()
  })
  it('never treats tool definitions or input history as actions of this request', () => {
    const activity = buildOpenClawActivity([heartbeat()])[0]
    expect(activity.requestedTools).toEqual([])
    expect(activity.heartbeatAcknowledged).toBe(true)
  })
  it('extracts names only from recorded response calls and does not assert success', () => {
    const turn = heartbeat()
    turn.model_response = ''
    turn.request_details!.response_context = [
      { role: 'assistant', kind: 'tool_use', content: '{"type":"tool_use","name":"session_status","input":{}}', position: 0, truncated: false },
      { role: 'assistant', kind: 'tool_use', content: '[{"function":{"name":"read"}}]', position: 1, truncated: false },
    ]
    const activity = buildOpenClawActivity([turn])[0]
    expect(activity.requestedTools).toEqual(['session_status', 'read'])
    expect(activity.heartbeatAcknowledged).toBe(false)
  })
  it('labels incomplete calls without inventing a tool name', () => {
    const turn = heartbeat()
    turn.request_details!.response_context = [{ role: 'assistant', kind: 'tool_use_delta', content: '{"partial_json":"{\\"path\\":"}', position: 0, truncated: false }]
    const activity = buildOpenClawActivity([turn])[0]
    expect(activity.fragmentedCalls).toBe(true)
    expect(activity.requestedTools).toEqual([])
  })
  it('does not identify ordinary user mentions as OpenClaw runtime evidence', () => {
    const turn = heartbeat()
    turn.request_details!.context = []
    turn.request_details!.messages[0].content = 'Explain OpenClaw to me'
    expect(buildOpenClawActivity([turn])).toEqual([])
  })
})
