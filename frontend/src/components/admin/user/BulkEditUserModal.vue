<template>
  <BaseDialog
    :show="show"
    :title="t('admin.users.bulkLimits.title')"
    width="normal"
    @close="emit('close')"
  >
    <form id="bulk-edit-user-limits-form" class="space-y-5" @submit.prevent="handleSubmit">
      <p class="text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.users.bulkLimits.selectedCount', { count: selectedIds.length }) }}
      </p>

      <div class="divide-y divide-gray-200 border-y border-gray-200 dark:divide-dark-700 dark:border-dark-700">
        <div class="space-y-3 py-4">
          <div class="flex items-center justify-between gap-4">
            <label for="bulk-concurrency" class="input-label mb-0">
              {{ t('admin.users.columns.concurrency') }}
            </label>
            <Toggle
              v-model="enableConcurrency"
              :aria-label="t('admin.users.bulkLimits.enableConcurrency')"
              data-test="enable-concurrency"
            />
          </div>
          <input
            v-if="enableConcurrency"
            id="bulk-concurrency"
            v-model="concurrencyValue"
            type="number"
            min="0"
            step="1"
            class="input"
            data-test="concurrency-input"
          />
        </div>

        <div class="space-y-3 py-4">
          <div class="flex items-center justify-between gap-4">
            <label for="bulk-rpm-limit" class="input-label mb-0">
              {{ t('admin.users.form.rpmLimit') }}
            </label>
            <Toggle
              v-model="enableRPMLimit"
              :aria-label="t('admin.users.bulkLimits.enableRPMLimit')"
              data-test="enable-rpm-limit"
            />
          </div>
          <div v-if="enableRPMLimit">
            <input
              id="bulk-rpm-limit"
              v-model="rpmLimitValue"
              type="number"
              min="0"
              step="1"
              class="input"
              data-test="rpm-limit-input"
            />
            <p v-if="parsedRPMLimit === 0" class="input-hint">
              {{ t('admin.users.bulkLimits.unlimited') }}
            </p>
          </div>
        </div>

        <div class="space-y-3 py-4">
          <div class="flex items-center justify-between gap-4">
            <label for="bulk-token-limit-1d" class="input-label mb-0">
              {{ t('admin.users.bulkLimits.gptTokenLimits') }}
            </label>
            <Toggle
              v-model="enableTokenLimits"
              :aria-label="t('admin.users.bulkLimits.enableTokenLimits')"
              data-test="enable-token-limits"
            />
          </div>
          <div v-if="enableTokenLimits" class="grid gap-3 sm:grid-cols-3">
            <div>
              <label for="bulk-token-limit-1d" class="input-label">
                {{ t('admin.users.bulkLimits.tokenLimit1d') }}
              </label>
              <input
                id="bulk-token-limit-1d"
                v-model="tokenLimit1dValue"
                type="number"
                min="0"
                step="1"
                class="input"
                :placeholder="t('admin.users.bulkLimits.leaveUnchanged')"
                data-test="token-limit-1d-input"
              />
            </div>
            <div>
              <label for="bulk-token-limit-7d" class="input-label">
                {{ t('admin.users.bulkLimits.tokenLimit7d') }}
              </label>
              <input
                id="bulk-token-limit-7d"
                v-model="tokenLimit7dValue"
                type="number"
                min="0"
                step="1"
                class="input"
                :placeholder="t('admin.users.bulkLimits.leaveUnchanged')"
                data-test="token-limit-7d-input"
              />
            </div>
            <div>
              <label for="bulk-token-limit-30d" class="input-label">
                {{ t('admin.users.bulkLimits.tokenLimit30d') }}
              </label>
              <input
                id="bulk-token-limit-30d"
                v-model="tokenLimit30dValue"
                type="number"
                min="0"
                step="1"
                class="input"
                :placeholder="t('admin.users.bulkLimits.leaveUnchanged')"
                data-test="token-limit-30d-input"
              />
            </div>
          </div>
          <p v-if="enableTokenLimits" class="input-hint">
            {{ t('admin.users.bulkLimits.tokenLimitsHint') }}
          </p>
          <div class="flex flex-col gap-3 rounded-lg border border-amber-200 bg-amber-50 p-3 dark:border-amber-900/60 dark:bg-amber-950/20 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <p class="text-sm font-medium text-gray-800 dark:text-gray-200">
                {{ t('admin.users.bulkLimits.resetTokenQuota') }}
              </p>
              <p class="mt-1 text-xs text-gray-600 dark:text-gray-400">
                {{ t('admin.users.bulkLimits.resetTokenQuotaHint') }}
              </p>
            </div>
            <button
              type="button"
              class="btn btn-secondary shrink-0"
              :disabled="!canResetTokenQuota"
              data-test="reset-token-quota"
              @click="handleResetTokenQuota"
            >
              {{ resettingTokenQuota
                ? t('admin.users.bulkLimits.resettingTokenQuota')
                : t('admin.users.bulkLimits.resetTokenQuota') }}
            </button>
          </div>
        </div>

        <div class="space-y-3 py-4">
          <div class="flex items-center justify-between gap-4">
            <label for="bulk-platform-quota-platform" class="input-label mb-0">
              {{ t('admin.users.bulkLimits.platformUsageTitle') }}
            </label>
            <Toggle
              v-model="enablePlatformUsage"
              :aria-label="t('admin.users.bulkLimits.enablePlatformUsage')"
              data-test="enable-platform-usage"
            />
          </div>
          <div v-if="enablePlatformUsage" class="grid gap-3 sm:grid-cols-3">
            <div>
              <label for="bulk-platform-quota-platform" class="input-label">
                {{ t('admin.users.platformQuota.columns.platform') }}
              </label>
              <select
                id="bulk-platform-quota-platform"
                v-model="platformUsagePlatform"
                class="input"
                data-test="platform-usage-platform"
              >
                <option v-for="platform in PLATFORM_OPTIONS" :key="platform" :value="platform">
                  {{ platform }}
                </option>
              </select>
            </div>
            <div>
              <label for="bulk-daily-usage" class="input-label">
                {{ t('admin.users.bulkLimits.dailyUsage') }}
              </label>
              <input
                id="bulk-daily-usage"
                v-model="dailyUsageValue"
                type="number"
                min="0"
                step="0.000001"
                class="input"
                :placeholder="t('admin.users.bulkLimits.leaveUnchanged')"
                data-test="daily-usage-input"
              />
            </div>
            <div>
              <label for="bulk-weekly-usage" class="input-label">
                {{ t('admin.users.bulkLimits.weeklyUsage') }}
              </label>
              <input
                id="bulk-weekly-usage"
                v-model="weeklyUsageValue"
                type="number"
                min="0"
                step="0.000001"
                class="input"
                :placeholder="t('admin.users.bulkLimits.leaveUnchanged')"
                data-test="weekly-usage-input"
              />
            </div>
          </div>
          <p v-if="enablePlatformUsage" class="input-hint">
            {{ t('admin.users.bulkLimits.platformUsageHint') }}
          </p>
        </div>
      </div>

      <p v-if="hasInvalidValue" class="text-sm text-red-600 dark:text-red-400">
        {{ t('admin.users.bulkLimits.invalidValue') }}
      </p>
      <p v-if="selectionTooLarge" class="text-sm text-red-600 dark:text-red-400">
        {{ t('admin.users.bulkLimits.selectionLimit', { max: MAX_BATCH_USER_IDS }) }}
      </p>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="emit('close')">
          {{ t('common.cancel') }}
        </button>
        <button
          type="submit"
          form="bulk-edit-user-limits-form"
          class="btn btn-primary"
          :disabled="!canSubmit"
          data-test="submit"
        >
          {{ submitting ? t('admin.users.bulkLimits.applying') : t('admin.users.bulkLimits.apply') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type {
  BatchAdjustPlatformQuotaUsageRequest,
  BatchUpdateUserLimitsRequest,
  PlatformQuotaPlatform
} from '@/api/admin/users'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Toggle from '@/components/common/Toggle.vue'

const props = defineProps<{
  show: boolean
  selectedIds: number[]
}>()

const emit = defineEmits<{
  close: []
  success: [affected: number]
}>()

const { t } = useI18n()
const appStore = useAppStore()
const enableConcurrency = ref(false)
const enableRPMLimit = ref(false)
const enableTokenLimits = ref(false)
const enablePlatformUsage = ref(false)
const concurrencyValue = ref<string | number>('')
const rpmLimitValue = ref<string | number>('')
const tokenLimit1dValue = ref<string | number>('')
const tokenLimit7dValue = ref<string | number>('')
const tokenLimit30dValue = ref<string | number>('')
const dailyUsageValue = ref<string | number>('')
const weeklyUsageValue = ref<string | number>('')
const platformUsagePlatform = ref<PlatformQuotaPlatform>('openai')
const submitting = ref(false)
const resettingTokenQuota = ref(false)
const MAX_BATCH_USER_IDS = 500
const PLATFORM_OPTIONS: PlatformQuotaPlatform[] = ['anthropic', 'openai', 'gemini', 'antigravity', 'grok']

const parseLimit = (value: string | number): number | null | undefined => {
  const trimmed = String(value).trim()
  if (!trimmed) return undefined
  const parsed = Number(trimmed)
  if (!Number.isInteger(parsed) || parsed < 0) return null
  return parsed
}

const parseUsage = (value: string | number): number | null | undefined => {
  const trimmed = String(value).trim()
  if (!trimmed) return undefined
  const parsed = Number(trimmed)
  if (!Number.isFinite(parsed) || parsed < 0) return null
  return parsed
}

const parsedConcurrency = computed(() =>
  enableConcurrency.value ? parseLimit(concurrencyValue.value) : undefined
)
const parsedRPMLimit = computed(() =>
  enableRPMLimit.value ? parseLimit(rpmLimitValue.value) : undefined
)
const parsedTokenLimit1d = computed(() =>
  enableTokenLimits.value ? parseLimit(tokenLimit1dValue.value) : undefined
)
const parsedTokenLimit7d = computed(() =>
  enableTokenLimits.value ? parseLimit(tokenLimit7dValue.value) : undefined
)
const parsedTokenLimit30d = computed(() =>
  enableTokenLimits.value ? parseLimit(tokenLimit30dValue.value) : undefined
)
const parsedDailyUsage = computed(() =>
  enablePlatformUsage.value ? parseUsage(dailyUsageValue.value) : undefined
)
const parsedWeeklyUsage = computed(() =>
  enablePlatformUsage.value ? parseUsage(weeklyUsageValue.value) : undefined
)
const hasInvalidValue = computed(() =>
  parsedConcurrency.value === null
  || parsedRPMLimit.value === null
  || parsedTokenLimit1d.value === null
  || parsedTokenLimit7d.value === null
  || parsedTokenLimit30d.value === null
  || parsedDailyUsage.value === null
  || parsedWeeklyUsage.value === null
)
const hasUpdate = computed(() =>
  (parsedConcurrency.value !== undefined && parsedConcurrency.value !== null)
  || (parsedRPMLimit.value !== undefined && parsedRPMLimit.value !== null)
  || (parsedTokenLimit1d.value !== undefined && parsedTokenLimit1d.value !== null)
  || (parsedTokenLimit7d.value !== undefined && parsedTokenLimit7d.value !== null)
  || (parsedTokenLimit30d.value !== undefined && parsedTokenLimit30d.value !== null)
  || (parsedDailyUsage.value !== undefined && parsedDailyUsage.value !== null)
  || (parsedWeeklyUsage.value !== undefined && parsedWeeklyUsage.value !== null)
)
const selectionTooLarge = computed(() => props.selectedIds.length > MAX_BATCH_USER_IDS)
const canSubmit = computed(() =>
  props.selectedIds.length > 0
  && !selectionTooLarge.value
  && hasUpdate.value
  && !hasInvalidValue.value
  && !submitting.value
  && !resettingTokenQuota.value
)
const canResetTokenQuota = computed(() =>
  props.selectedIds.length > 0
  && !selectionTooLarge.value
  && !submitting.value
  && !resettingTokenQuota.value
)

const reset = () => {
  enableConcurrency.value = false
  enableRPMLimit.value = false
  enableTokenLimits.value = false
  enablePlatformUsage.value = false
  concurrencyValue.value = ''
  rpmLimitValue.value = ''
  tokenLimit1dValue.value = ''
  tokenLimit7dValue.value = ''
  tokenLimit30dValue.value = ''
  dailyUsageValue.value = ''
  weeklyUsageValue.value = ''
  platformUsagePlatform.value = 'openai'
  submitting.value = false
  resettingTokenQuota.value = false
}

watch(
  () => props.show,
  (show) => {
    if (show) reset()
  }
)

const handleSubmit = async () => {
  if (!canSubmit.value) return

  const fields: string[] = []
  const limitRequest: BatchUpdateUserLimitsRequest = {
    user_ids: [...props.selectedIds],
    all: false
  }
  if (parsedConcurrency.value !== undefined && parsedConcurrency.value !== null) {
    limitRequest.concurrency = parsedConcurrency.value
    fields.push(
      t('admin.users.bulkLimits.concurrencyValue', { value: parsedConcurrency.value })
    )
  }
  if (parsedRPMLimit.value !== undefined && parsedRPMLimit.value !== null) {
    limitRequest.rpm_limit = parsedRPMLimit.value
    fields.push(
      parsedRPMLimit.value === 0
        ? t('admin.users.bulkLimits.rpmUnlimitedValue')
        : t('admin.users.bulkLimits.rpmValue', { value: parsedRPMLimit.value })
    )
  }
  if (parsedTokenLimit1d.value !== undefined && parsedTokenLimit1d.value !== null) {
    limitRequest.token_limit_1d = parsedTokenLimit1d.value
    fields.push(t('admin.users.bulkLimits.tokenLimit1dValue', { value: parsedTokenLimit1d.value }))
  }
  if (parsedTokenLimit7d.value !== undefined && parsedTokenLimit7d.value !== null) {
    limitRequest.token_limit_7d = parsedTokenLimit7d.value
    fields.push(t('admin.users.bulkLimits.tokenLimit7dValue', { value: parsedTokenLimit7d.value }))
  }
  if (parsedTokenLimit30d.value !== undefined && parsedTokenLimit30d.value !== null) {
    limitRequest.token_limit_30d = parsedTokenLimit30d.value
    fields.push(t('admin.users.bulkLimits.tokenLimit30dValue', { value: parsedTokenLimit30d.value }))
  }
  const usageRequest: BatchAdjustPlatformQuotaUsageRequest = {
    user_ids: [...props.selectedIds],
    all: false,
    platform: platformUsagePlatform.value
  }
  if (parsedDailyUsage.value !== undefined && parsedDailyUsage.value !== null) {
    usageRequest.daily_usage_usd = parsedDailyUsage.value
    fields.push(t('admin.users.bulkLimits.dailyUsageValue', {
      platform: platformUsagePlatform.value,
      value: parsedDailyUsage.value
    }))
  }
  if (parsedWeeklyUsage.value !== undefined && parsedWeeklyUsage.value !== null) {
    usageRequest.weekly_usage_usd = parsedWeeklyUsage.value
    fields.push(t('admin.users.bulkLimits.weeklyUsageValue', {
      platform: platformUsagePlatform.value,
      value: parsedWeeklyUsage.value
    }))
  }

  const confirmed = window.confirm(
    t('admin.users.bulkLimits.confirm', {
      count: props.selectedIds.length,
      fields: fields.join(', ')
    })
  )
  if (!confirmed) return

  submitting.value = true
  try {
    let affected = 0
    if (
      limitRequest.concurrency !== undefined
      || limitRequest.rpm_limit !== undefined
      || limitRequest.token_limit_1d !== undefined
      || limitRequest.token_limit_7d !== undefined
      || limitRequest.token_limit_30d !== undefined
    ) {
      const result = await adminAPI.users.batchUpdateLimits(limitRequest)
      affected = Math.max(affected, result.affected)
    }
    if (usageRequest.daily_usage_usd !== undefined || usageRequest.weekly_usage_usd !== undefined) {
      const result = await adminAPI.users.batchAdjustPlatformQuotaUsage(usageRequest)
      affected = Math.max(affected, result.affected)
    }
    appStore.showSuccess(
      t('admin.users.bulkLimits.success', { count: affected })
    )
    emit('success', affected)
    emit('close')
  } catch (error: any) {
    appStore.showError(
      error.response?.data?.message
      || error.response?.data?.detail
      || t('admin.users.bulkLimits.failed')
    )
  } finally {
    submitting.value = false
  }
}

const handleResetTokenQuota = async () => {
  if (!canResetTokenQuota.value) return

  const confirmed = window.confirm(
    t('admin.users.bulkLimits.resetTokenQuotaConfirm', {
      count: props.selectedIds.length
    })
  )
  if (!confirmed) return

  resettingTokenQuota.value = true
  try {
    const result = await adminAPI.users.batchUpdateLimits({
      user_ids: [...props.selectedIds],
      all: false,
      reset_token_quota: true
    })
    appStore.showSuccess(
      t('admin.users.bulkLimits.resetTokenQuotaSuccess', { count: result.affected })
    )
    emit('success', result.affected)
    emit('close')
  } catch (error: any) {
    appStore.showError(
      error.response?.data?.message
      || error.response?.data?.detail
      || t('admin.users.bulkLimits.resetTokenQuotaFailed')
    )
  } finally {
    resettingTokenQuota.value = false
  }
}
</script>
