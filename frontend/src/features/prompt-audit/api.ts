import { apiClient } from '@/api/client'
import type {
  PromptAuditConfig,
  PromptAuditEvent,
  PromptAuditGroup,
  PromptAuditRuntime,
  PromptAuditUpdateRequest,
  PromptDeletePreview,
  PromptDeleteResult,
  PromptEventFilters,
  PromptEventPage,
  PromptProbeResult,
  PromptAuditEndpointDraft,
  ConversationFilters,
  ConversationPage,
  ConversationReviewRun,
  ConversationReviewRunPage,
  ConversationSession,
} from './types'
import { conversationQueryParams, eventFilterPayload, eventQueryParams } from './viewModel'

const basePath = '/admin/prompt-audit'

export async function getConfig(): Promise<PromptAuditConfig> {
  const { data } = await apiClient.get<PromptAuditConfig>(`${basePath}/config`)
  return data
}

export async function updateConfig(payload: PromptAuditUpdateRequest): Promise<PromptAuditConfig> {
  const { data } = await apiClient.put<PromptAuditConfig>(`${basePath}/config`, payload)
  return data
}

export async function probeEndpoint(endpoint: PromptAuditEndpointDraft): Promise<PromptProbeResult> {
  const { data } = await apiClient.post<PromptProbeResult>(`${basePath}/endpoints/probe`, {
    endpoint: {
      id: endpoint.id,
      name: endpoint.name,
      protocol: 'openai_compatible',
      base_url: endpoint.base_url,
      model: endpoint.model,
      token: endpoint.token || undefined,
      timeout_ms: endpoint.timeout_ms,
      input_limit: endpoint.input_limit,
      enabled: endpoint.enabled,
    },
  })
  return data
}

export async function getRuntime(): Promise<PromptAuditRuntime> {
  const { data } = await apiClient.get<PromptAuditRuntime>(`${basePath}/runtime`)
  return data
}

export async function listEvents(
  filters: PromptEventFilters,
  page: number,
  pageSize: number,
): Promise<PromptEventPage> {
  const { data } = await apiClient.get<PromptEventPage>(`${basePath}/events`, {
    params: { page, page_size: pageSize, ...eventQueryParams(filters) },
  })
  return data
}

export async function getEvent(id: number): Promise<PromptAuditEvent> {
  const { data } = await apiClient.get<PromptAuditEvent>(`${basePath}/events/${id}`)
  return data
}

export async function deleteEvent(id: number): Promise<PromptDeleteResult> {
  const { data } = await apiClient.delete<PromptDeleteResult>(`${basePath}/events/${id}`)
  return data
}

export async function batchDeleteEvents(ids: number[]): Promise<PromptDeleteResult> {
  const { data } = await apiClient.post<PromptDeleteResult>(`${basePath}/events/batch-delete`, { ids })
  return data
}

export async function previewDelete(filters: PromptEventFilters): Promise<PromptDeletePreview> {
  const { data } = await apiClient.post<PromptDeletePreview>(
    `${basePath}/events/delete-preview`,
    eventFilterPayload(filters),
  )
  return data
}

export async function deleteEventsByFilter(
  filters: PromptEventFilters,
  preview: PromptDeletePreview,
): Promise<PromptDeleteResult> {
  const { data } = await apiClient.post<PromptDeleteResult>(`${basePath}/events/delete-by-filter`, {
    filter: eventFilterPayload(filters),
    snapshot_max_id: preview.snapshot_max_id,
    filter_hash: preview.filter_hash,
    confirmation_token: preview.confirmation_token,
    confirm: true,
  })
  return data
}

export async function listGroups(): Promise<PromptAuditGroup[]> {
  const { data } = await apiClient.get<PromptAuditGroup[]>('/admin/groups/all', {
    params: { include_inactive: true },
  })
  return data
}

export async function listConversations(filters: ConversationFilters, page: number, pageSize: number): Promise<ConversationPage> {
  const { data } = await apiClient.get<ConversationPage>(`${basePath}/conversations`, {
    params: { page, page_size: pageSize, ...conversationQueryParams(filters) },
  })
  return data
}

export async function getConversation(id: number): Promise<ConversationSession> {
  const { data } = await apiClient.get<ConversationSession>(`${basePath}/conversations/${id}`)
  return data
}

export async function deleteConversation(id: number): Promise<{ deleted_sessions: number; deleted_turns: number }> {
  const { data } = await apiClient.delete<{ deleted_sessions: number; deleted_turns: number }>(`${basePath}/conversations/${id}`)
  return data
}

export async function listConversationReviewRuns(page: number, pageSize: number): Promise<ConversationReviewRunPage> {
  const { data } = await apiClient.get<ConversationReviewRunPage>(`${basePath}/conversation-review/runs`, {
    params: { page, page_size: pageSize },
  })
  return data
}

export async function runConversationReview(): Promise<ConversationReviewRun> {
  const { data } = await apiClient.post<ConversationReviewRun>(`${basePath}/conversation-review/run`)
  return data
}

export const promptAuditAPI = {
  getConfig,
  updateConfig,
  probeEndpoint,
  getRuntime,
  listEvents,
  getEvent,
  deleteEvent,
  batchDeleteEvents,
  previewDelete,
  deleteEventsByFilter,
  listGroups,
  listConversations,
  getConversation,
  deleteConversation,
  listConversationReviewRuns,
  runConversationReview,
}

export default promptAuditAPI
