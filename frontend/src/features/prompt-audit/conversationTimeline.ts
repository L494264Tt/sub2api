import type { ConversationMessage, ConversationTurn } from './types'

export interface ConversationTimelineEntry {
  turn: ConversationTurn
  messages: ConversationMessage[]
}

// Clients resend prior messages on each request. Remove only an exact overlap
// with the already displayed history; equal text at a different role is new.
export function buildConversationTimeline(turns: ConversationTurn[]): ConversationTimelineEntry[] {
  const result: ConversationTimelineEntry[] = []
  let previous: ConversationMessage[] = []
  for (const turn of [...turns].sort((a, b) => a.captured_at.localeCompare(b.captured_at) || a.id - b.id)) {
    if (turn.request_kind === 'auxiliary') continue
    const incoming = turn.request_details?.messages || []
    let overlap = Math.min(previous.length, incoming.length)
    for (; overlap > 0; overlap--) {
      const suffix = previous.slice(-overlap)
      if (suffix.every((message, index) => message.role === incoming[index].role &&
        message.content.trim() === incoming[index].content.trim() && !message.truncated && !incoming[index].truncated)) break
    }
    result.push({ turn, messages: incoming.slice(overlap) })
    previous = [...incoming]
    if (turn.model_response && !turn.response_truncated) {
      previous.push({ role: 'assistant', content: turn.model_response, kind: 'text', position: -1, truncated: false })
    }
  }
  return result
}
