import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import BulkEditUserModal from '../BulkEditUserModal.vue'

const { batchUpdateLimits, batchAdjustPlatformQuotaUsage, showSuccess, showError } = vi.hoisted(() => ({
  batchUpdateLimits: vi.fn(),
  batchAdjustPlatformQuotaUsage: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      batchUpdateLimits,
      batchAdjustPlatformQuotaUsage
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess,
    showError
  })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}:${JSON.stringify(params)}` : key
  })
}))

const mountModal = () => mount(BulkEditUserModal, {
  props: {
    show: true,
    selectedIds: [4, 7]
  },
  global: {
    stubs: {
      BaseDialog: {
        props: ['show', 'title'],
        emits: ['close'],
        template: '<div v-if="show"><slot /><slot name="footer" /></div>'
      }
    }
  }
})

describe('BulkEditUserModal', () => {
  beforeEach(() => {
    batchUpdateLimits.mockReset()
    batchAdjustPlatformQuotaUsage.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    batchUpdateLimits.mockResolvedValue({ affected: 2 })
    batchAdjustPlatformQuotaUsage.mockResolvedValue({ affected: 2, platform: 'openai' })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('disables submission until at least one enabled field has a value', async () => {
    const wrapper = mountModal()

    expect(wrapper.get('[data-test="submit"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-test="enable-concurrency"]').trigger('click')
    expect(wrapper.get('[data-test="submit"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-test="concurrency-input"]').setValue('5')
    expect(wrapper.get('[data-test="submit"]').attributes('disabled')).toBeUndefined()
  })

  it('disables submission when more than 500 users are selected', async () => {
    const wrapper = mountModal()
    await wrapper.setProps({ selectedIds: Array.from({ length: 501 }, (_, index) => index + 1) })
    await wrapper.get('[data-test="enable-concurrency"]').trigger('click')
    await wrapper.get('[data-test="concurrency-input"]').setValue('5')

    expect(wrapper.text()).toContain('admin.users.bulkLimits.selectionLimit')
    expect(wrapper.get('[data-test="submit"]').attributes('disabled')).toBeDefined()
  })

  it('submits only the enabled RPM field and preserves zero as unlimited', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mountModal()

    await wrapper.get('[data-test="enable-rpm-limit"]').trigger('click')
    await wrapper.get('[data-test="rpm-limit-input"]').setValue('0')
    expect(wrapper.text()).toContain('admin.users.bulkLimits.unlimited')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(batchUpdateLimits).toHaveBeenCalledWith({
      user_ids: [4, 7],
      all: false,
      rpm_limit: 0
    })
    expect(confirm).toHaveBeenCalledWith(
      expect.stringContaining('admin.users.bulkLimits.rpmUnlimitedValue')
    )
    expect(wrapper.emitted('success')).toEqual([[2]])
  })

  it('omits disabled fields from the request', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mountModal()

    await wrapper.get('[data-test="enable-concurrency"]').trigger('click')
    await wrapper.get('[data-test="concurrency-input"]').setValue('9')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(batchUpdateLimits).toHaveBeenCalledWith({
      user_ids: [4, 7],
      all: false,
      concurrency: 9
    })
  })

  it('submits selected GPT token limits and preserves zero as unlimited', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mountModal()

    await wrapper.get('[data-test="enable-token-limits"]').trigger('click')
    await wrapper.get('[data-test="token-limit-1d-input"]').setValue('100000000')
    await wrapper.get('[data-test="token-limit-7d-input"]').setValue('400000000')
    await wrapper.get('[data-test="token-limit-30d-input"]').setValue('0')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(batchUpdateLimits).toHaveBeenCalledWith({
      user_ids: [4, 7],
      all: false,
      token_limit_1d: 100000000,
      token_limit_7d: 400000000,
      token_limit_30d: 0
    })
    expect(confirm).toHaveBeenCalledWith(
      expect.stringContaining('admin.users.bulkLimits.tokenLimit7dValue')
    )
    expect(wrapper.emitted('success')).toEqual([[2]])
  })

  it('submits platform usage without calling the limits endpoint', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mountModal()

    await wrapper.get('[data-test="enable-platform-usage"]').trigger('click')
    await wrapper.get('[data-test="platform-usage-platform"]').setValue('gemini')
    await wrapper.get('[data-test="daily-usage-input"]').setValue('1.25')
    await wrapper.get('[data-test="weekly-usage-input"]').setValue('3.5')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(batchUpdateLimits).not.toHaveBeenCalled()
    expect(batchAdjustPlatformQuotaUsage).toHaveBeenCalledWith({
      user_ids: [4, 7],
      all: false,
      platform: 'gemini',
      daily_usage_usd: 1.25,
      weekly_usage_usd: 3.5
    })
    expect(confirm).toHaveBeenCalledWith(
      expect.stringContaining('admin.users.bulkLimits.weeklyUsageValue')
    )
    expect(wrapper.emitted('success')).toEqual([[2]])
  })

  it('resets GPT token quota usage from the current time', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mountModal()

    await wrapper.get('[data-test="reset-token-quota"]').trigger('click')
    await flushPromises()

    expect(batchUpdateLimits).toHaveBeenCalledWith({
      user_ids: [4, 7],
      all: false,
      reset_token_quota: true
    })
    expect(confirm).toHaveBeenCalledWith(
      expect.stringContaining('admin.users.bulkLimits.resetTokenQuotaConfirm')
    )
    expect(showSuccess).toHaveBeenCalledWith(
      expect.stringContaining('admin.users.bulkLimits.resetTokenQuotaSuccess')
    )
    expect(wrapper.emitted('success')).toEqual([[2]])
  })

  it('does not reset GPT token quota usage when confirmation is cancelled', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    const wrapper = mountModal()

    await wrapper.get('[data-test="reset-token-quota"]').trigger('click')
    await flushPromises()

    expect(batchUpdateLimits).not.toHaveBeenCalled()
  })

  it('does not call the API when overwrite confirmation is cancelled', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    const wrapper = mountModal()

    await wrapper.get('[data-test="enable-concurrency"]').trigger('click')
    await wrapper.get('[data-test="concurrency-input"]').setValue('9')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(batchUpdateLimits).not.toHaveBeenCalled()
    expect(batchAdjustPlatformQuotaUsage).not.toHaveBeenCalled()
  })
})
