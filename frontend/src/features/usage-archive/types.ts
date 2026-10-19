export interface ArchiveSessionSummary {
  id: number
  session_key: string
  user_id: number
  api_key_id: number
  group_id?: number | null
  provider: string
  model: string
  first_seen_at: string
  last_seen_at: string
  turn_count: number
  content_bytes: number
  continuity_confidence: string
}

export interface ArchiveMessage {
  id: number
  role: string
  source: string
  ordinal: number
  content: string
  content_hash: string
  content_bytes: number
  content_runes: number
  truncated: boolean
  content_expired: boolean
}

export interface ArchiveTurn {
  id: number
  request_id: string
  parent_turn_id?: number | null
  sequence_no: number
  conversation_id: string
  previous_response_id: string
  upstream_response_id: string
  endpoint: string
  protocol: string
  provider: string
  model: string
  status_code: number
  duration_ms?: number | null
  first_token_ms?: number | null
  continuity_source: string
  continuity_confidence: string
  request_truncated: boolean
  response_truncated: boolean
  captured_at: string
  messages: ArchiveMessage[]
}

export interface ArchiveSession extends ArchiveSessionSummary {
  turns: ArchiveTurn[]
}

export interface ArchiveSessionPage {
  items: ArchiveSessionSummary[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface ArchiveStats {
  sessions: number
  turns: number
  messages: number
  content_bytes: number
  user_messages: number
  prompt_messages: number
  assistant_messages: number
}

export interface ArchiveFilters {
  user_id: string
  api_key_id: string
  group_id: string
  provider: string
  model: string
  request_id: string
  session_key: string
}
