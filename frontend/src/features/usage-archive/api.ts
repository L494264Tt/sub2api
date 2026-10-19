import { apiClient } from '@/api/client'
import type { ArchiveFilters, ArchiveSession, ArchiveSessionPage, ArchiveStats } from './types'

const basePath = '/admin/usage-archive'

function params(filters: ArchiveFilters, page: number, pageSize: number) {
  const result: Record<string, string | number> = { page, page_size: pageSize }
  for (const key of Object.keys(filters) as Array<keyof ArchiveFilters>) {
    const value = filters[key].trim()
    if (value) result[key] = value
  }
  return result
}

export async function listSessions(filters: ArchiveFilters, page: number, pageSize: number) {
  const { data } = await apiClient.get<ArchiveSessionPage>(`${basePath}/sessions`, { params: params(filters, page, pageSize) })
  return data
}

export async function getSession(id: number) {
  const { data } = await apiClient.get<ArchiveSession>(`${basePath}/sessions/${id}`)
  return data
}

export async function getStats() {
  const { data } = await apiClient.get<ArchiveStats>(`${basePath}/stats`)
  return data
}

export async function deleteSessionContent(id: number) {
  const { data } = await apiClient.delete<{ deleted_messages: number }>(`${basePath}/sessions/${id}/content`)
  return data
}

export const usageArchiveAPI = { listSessions, getSession, getStats, deleteSessionContent }
export default usageArchiveAPI
