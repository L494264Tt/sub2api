import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({
  post: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    post,
  },
}))

import {
  batchAdjustPlatformQuotaUsage,
  batchUpdateLimits,
  bindUserAuthIdentity,
  type AdminBindAuthIdentityRequest,
  type AdminBoundAuthIdentity,
  type BatchAdjustPlatformQuotaUsageRequest,
  type BatchAdjustPlatformQuotaUsageResponse,
  type BatchUpdateUserLimitsRequest,
  type BatchUpdateUserLimitsResponse,
} from '@/api/admin/users'

type Assert<T extends true> = T
type IsExact<T, U> = (
  (<G>() => G extends T ? 1 : 2) extends (<G>() => G extends U ? 1 : 2)
    ? ((<G>() => G extends U ? 1 : 2) extends (<G>() => G extends T ? 1 : 2) ? true : false)
    : false
)

type ExpectedAdminBindAuthIdentityRequest = {
  provider_type: string
  provider_key: string
  provider_subject: string
  issuer?: string
  metadata?: Record<string, unknown>
  channel?: {
    channel: string
    channel_app_id: string
    channel_subject: string
    metadata?: Record<string, unknown>
  }
}

type ExpectedAdminBoundAuthIdentity = {
  user_id: number
  provider_type: string
  provider_key: string
  provider_subject: string
  verified_at?: string | null
  issuer?: string | null
  metadata: Record<string, unknown> | null
  created_at: string
  updated_at: string
  channel?: {
    channel: string
    channel_app_id: string
    channel_subject: string
    metadata: Record<string, unknown> | null
    created_at: string
    updated_at: string
  } | null
}

const requestContractExact: Assert<
  IsExact<AdminBindAuthIdentityRequest, ExpectedAdminBindAuthIdentityRequest>
> = true
const responseContractExact: Assert<
  IsExact<AdminBoundAuthIdentity, ExpectedAdminBoundAuthIdentity>
> = true
const batchRequestContractExact: Assert<
  IsExact<
    BatchUpdateUserLimitsRequest,
    {
      user_ids: number[]
      all?: boolean
      concurrency?: number
      rpm_limit?: number
      token_limit_1d?: number
      token_limit_7d?: number
      token_limit_30d?: number
	  reset_token_quota?: boolean
    }
  >
> = true
const batchResponseContractExact: Assert<
  IsExact<BatchUpdateUserLimitsResponse, { affected: number }>
> = true
const batchPlatformUsageRequestContractExact: Assert<
  IsExact<
    BatchAdjustPlatformQuotaUsageRequest,
    {
      user_ids: number[]
      all?: boolean
      platform: 'anthropic' | 'openai' | 'gemini' | 'antigravity' | 'grok'
      daily_usage_usd?: number
      weekly_usage_usd?: number
    }
  >
> = true
const batchPlatformUsageResponseContractExact: Assert<
  IsExact<
    BatchAdjustPlatformQuotaUsageResponse,
    {
      affected: number
      platform: 'anthropic' | 'openai' | 'gemini' | 'antigravity' | 'grok'
      daily_window_start?: string
      weekly_window_start?: string
    }
  >
> = true

describe('admin users api auth identity binding', () => {
  beforeEach(() => {
    post.mockReset()
  })

  it('posts the backend-compatible auth identity bind payload and returns the backend response shape', async () => {
    const payload: AdminBindAuthIdentityRequest = {
      provider_type: 'wechat',
      provider_key: 'wechat-main',
      provider_subject: 'union-123',
      metadata: { source: 'admin-repair' },
      channel: {
        channel: 'open',
        channel_app_id: 'wx-open',
        channel_subject: 'openid-123',
        metadata: { scene: 'migration' },
      },
    }

    const response: AdminBoundAuthIdentity = {
      user_id: 9,
      provider_type: 'wechat',
      provider_key: 'wechat-main',
      provider_subject: 'union-123',
      verified_at: '2026-04-22T00:00:00Z',
      issuer: null,
      metadata: { source: 'admin-repair' },
      created_at: '2026-04-22T00:00:00Z',
      updated_at: '2026-04-22T00:00:00Z',
      channel: {
        channel: 'open',
        channel_app_id: 'wx-open',
        channel_subject: 'openid-123',
        metadata: { scene: 'migration' },
        created_at: '2026-04-22T00:00:00Z',
        updated_at: '2026-04-22T00:00:00Z',
      },
    }
    post.mockResolvedValue({ data: response })

    const result = await bindUserAuthIdentity(9, payload)

    expect(post).toHaveBeenCalledWith('/admin/users/9/auth-identities', payload)
    expect(result).toEqual(response)
  })

  it('keeps bind auth identity request and response types aligned with the backend contract', () => {
    expect(requestContractExact).toBe(true)
    expect(responseContractExact).toBe(true)
  })

  it('posts batch limit updates once with only the supplied limit fields', async () => {
    const request: BatchUpdateUserLimitsRequest = {
      user_ids: [4, 7],
      all: false,
      token_limit_1d: 100_000_000,
      token_limit_7d: 400_000_000,
      token_limit_30d: 0,
    }
    post.mockResolvedValue({ data: { affected: 2 } satisfies BatchUpdateUserLimitsResponse })

    const result = await batchUpdateLimits(request)

    expect(post).toHaveBeenCalledWith('/admin/users/batch-limits', request)
    expect(result).toEqual({ affected: 2 })
    expect(batchRequestContractExact).toBe(true)
    expect(batchResponseContractExact).toBe(true)
  })

  it('posts batch platform usage updates once with only the supplied usage fields', async () => {
    const request: BatchAdjustPlatformQuotaUsageRequest = {
      user_ids: [4, 7],
      all: false,
      platform: 'openai',
      daily_usage_usd: 1.2,
    }
    const response: BatchAdjustPlatformQuotaUsageResponse = {
      affected: 2,
      platform: 'openai',
      daily_window_start: '2026-07-25T07:00:00+08:00',
      weekly_window_start: '2026-07-20T07:00:00+08:00',
    }
    post.mockResolvedValue({ data: response })

    const result = await batchAdjustPlatformQuotaUsage(request)

    expect(post).toHaveBeenCalledWith('/admin/users/batch-platform-quota-usage', request)
    expect(result).toEqual(response)
    expect(batchPlatformUsageRequestContractExact).toBe(true)
    expect(batchPlatformUsageResponseContractExact).toBe(true)
  })
})
