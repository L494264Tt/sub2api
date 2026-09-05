import { isOpenClawHeartbeat, isOpenClawTurn } from './conversationReading'
import type { ConversationTurn } from './types'

export interface OpenClawActivity {
  turn: ConversationTurn
  heartbeat: boolean
  heartbeatAcknowledged: boolean
  requestedTools: string[]
  fragmentedCalls: boolean
}

export function buildOpenClawActivity(turns: ConversationTurn[]): OpenClawActivity[] {
  return turns.filter(isOpenClawTurn).sort((a, b) => b.captured_at.localeCompare(a.captured_at) || b.id - a.id).map(turn => {
    const tools = new Set<string>()
    let fragmentedCalls = false
    // Only output from THIS request is evidence of a requested tool action.
    // Input tool definitions and prior tool results must not imply execution.
    for (const message of turn.request_details?.response_context || []) {
      if (!['tool_use', 'function_call', 'tool_use_delta'].includes(message.kind)) continue
      if (message.kind === 'tool_use_delta') fragmentedCalls = true
      try {
        const value = JSON.parse(message.content)
        for (const call of Array.isArray(value) ? value : [value]) {
          const name = call?.name || call?.function?.name
          if (typeof name === 'string' && name.trim()) tools.add(name.trim())
        }
      } catch { fragmentedCalls = true }
    }
    const heartbeat = isOpenClawHeartbeat(turn)
    return { turn, heartbeat, heartbeatAcknowledged: heartbeat && turn.model_response.trim() === 'HEARTBEAT_OK', requestedTools: [...tools], fragmentedCalls }
  })
}
