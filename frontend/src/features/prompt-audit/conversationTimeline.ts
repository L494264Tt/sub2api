import type { ConversationMessage, ConversationTurn } from './types'
import { isAuxiliaryTurn, readableMessages } from './conversationReading'

export interface ConversationTimelineEntry {
  turn: ConversationTurn
  messages: ConversationMessage[]
}

// Clients resend prior messages on each request. Remove only an exact overlap
// with the already displayed history; equal text at a different role is new.
export function buildConversationTimeline(turns: ConversationTurn[]): ConversationTimelineEntry[] {
  const result: ConversationTimelineEntry[] = []
  let previous: ConversationMessage[] = []
  let previousInput: ConversationMessage[] = []
  const equal = (a: ConversationMessage, b: ConversationMessage) => a.role === b.role && a.content.trim() === b.content.trim() && !a.truncated && !b.truncated
  for (const turn of [...turns].sort((a, b) => a.captured_at.localeCompare(b.captured_at) || a.id - b.id)) {
    if (isAuxiliaryTurn(turn)) continue
    const incoming = readableMessages(turn.request_details?.messages || [])
    let overlap = Math.min(previous.length, incoming.length)
    for (; overlap > 0; overlap--) {
      const suffix = previous.slice(-overlap)
      if (suffix.every((message, index) => equal(message, incoming[index]))) break
    }
    // Tool-only requests can resend the identical input snapshot even when an
    // intermediate captured response wasn't inserted into that input yet.
    let common = 0
    while (common < Math.min(previousInput.length, incoming.length) && equal(previousInput[common], incoming[common])) common++
    if (common === previousInput.length || common === incoming.length) overlap = Math.max(overlap, common)
    // A capped archive may retain a different partial message at the head on
    // every request. Anchor on the longest unchanged contiguous history block
    // rather than treating the entire shifted window as new dialogue.
    let longest = 0
    let end = 0
    let lengths = new Array(incoming.length + 1).fill(0) as number[]
    for (const previousMessage of previousInput) {
      const next = new Array(incoming.length + 1).fill(0) as number[]
      incoming.forEach((message, index) => {
        if (!equal(previousMessage, message)) return
        next[index + 1] = lengths[index] + 1
        if (next[index + 1] > longest || (next[index + 1] === longest && index + 1 > end)) {
          longest = next[index + 1]
          end = index + 1
        }
      })
      lengths = next
    }
    if (longest >= 2) overlap = Math.max(overlap, end)
    if (incoming.length < previousInput.length || turn.request_details?.omitted_messages) {
      let lengths = new Array(incoming.length + 1).fill(0) as number[]
      let matched = 0
      let matchedEnd = 0
      for (const oldMessage of previousInput) {
        const next = new Array(incoming.length + 1).fill(0) as number[]
        incoming.forEach((message, index) => {
          const same = equal(oldMessage, message)
          next[index + 1] = same ? lengths[index] + 1 : Math.max(lengths[index + 1], next[index])
          if (same && next[index + 1] > matched) { matched = next[index + 1]; matchedEnd = index + 1 }
        })
        lengths = next
      }
      if (matched >= 2) overlap = Math.max(overlap, matchedEnd)
    }
    const messages = incoming.slice(overlap)
    if (messages.length || turn.model_response.trim() || turn.request_details?.version !== 1) result.push({ turn, messages })
    previousInput = incoming
    previous = [...incoming]
    if (turn.model_response && !turn.response_truncated) {
      previous.push({ role: 'assistant', content: turn.model_response, kind: 'text', position: -1, truncated: false })
    }
  }
  return result
}
