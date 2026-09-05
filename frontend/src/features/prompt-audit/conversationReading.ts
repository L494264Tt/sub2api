import type { ConversationMessage, ConversationTurn } from './types'

const contextTags = ['environment_context', 'skills_instructions', 'skill', 'permissions', 'collaboration_mode']
const suggestionCheckPrefix = 'You are an expert at upholding safety and compliance standards for Codex ambient suggestions.'
const actionReviewPrefix = 'You are judging one planned coding-agent action.'

// Presentation-only cleanup: the original request remains in Per request.
// Match complete, anchored client envelopes; never remove a quoted tag or an
// incomplete block whose remaining text may contain the user's actual request.
export function dialogueText(text: string): string {
  let remaining = text.trim()
  if (remaining.startsWith('# AGENTS.md instructions for ') && remaining.includes('<INSTRUCTIONS>')) {
    const end = remaining.indexOf('</INSTRUCTIONS>')
    if (end < 0) return remaining
    remaining = remaining.slice(end + '</INSTRUCTIONS>'.length).trim()
  }
  let changed = true
  while (changed) {
    changed = false
    for (const tag of contextTags) {
      if (!remaining.startsWith(`<${tag}>`)) continue
      const end = remaining.indexOf(`</${tag}>`)
      if (end < 0) continue
      remaining = remaining.slice(end + tag.length + 3).trim()
      changed = true
      break
    }
  }
  return remaining
}

export function isAuxiliaryTurn(turn: ConversationTurn): boolean {
  if (turn.request_kind === 'auxiliary') return true
  const users = (turn.request_details?.messages || []).filter(message => message.role === 'user')
  if (turn.request_details?.context?.some(message => message.role === 'system' && message.content.trim().startsWith(actionReviewPrefix))) {
    try {
      const text = turn.model_response.trim().replace(/^(?:\*\*[^*\n]{1,120}\*\*\s*)+/, '')
      const result = JSON.parse(text)
      if (['allow', 'deny'].includes(result.outcome)) return true
    } catch { /* Keep unrecognized outputs in the dialogue. */ }
  }
  if (turn.model_response.trim() === 'HEARTBEAT_OK' && /^\[[^\n]+\] \[OpenClaw heartbeat poll\]$/.test(users.at(-1)?.content.trim() || '')) return true
  if (users.some(message => message.content.trim().startsWith(suggestionCheckPrefix))) {
    try {
      const result = JSON.parse(turn.model_response)
      return result && Array.isArray(result.exclude) && Object.keys(result).length === 1
    } catch { return false }
  }
  return false
}

export function readableMessages(messages: ConversationMessage[]): ConversationMessage[] {
  return messages.flatMap(message => {
    if (message.role !== 'user') return [message]
    const content = dialogueText(message.content)
    return content ? [{ ...message, content }] : []
  })
}

export function readingResponse(turn: ConversationTurn): string {
  const text = turn.model_response.trim()
  // Older captures concatenated adjacent reasoning-summary titles with output.
  // Only the reading view folds that recognizable prefix; raw records remain.
  if (turn.request_details?.response_context?.some(message => /reasoning|tool_use_delta/.test(message.kind))) {
    if (/^(?:\*\*[^*\n]{1,120}\*\*\s*)+$/.test(text)) return ''
    return text.replace(/^(?:\*\*[^*\n]{1,120}\*\*){2,}/, '').trim()
  }
  return text
}
