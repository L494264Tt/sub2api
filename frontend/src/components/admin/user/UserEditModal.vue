<template>
  <BaseDialog
    :show="show"
    :title="t('admin.users.editUser')"
    width="wide"
    @close="$emit('close')"
  >
    <form v-if="user" id="edit-user-form" @submit.prevent="handleUpdateUser" class="space-y-5">
      <div>
        <label class="input-label">{{ t('admin.users.email') }}</label>
        <input v-model="form.email" type="email" class="input" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.password') }}</label>
        <div class="flex gap-2">
          <div class="relative flex-1">
            <input v-model="form.password" type="text" class="input pr-10" :placeholder="t('admin.users.enterNewPassword')" />
            <button v-if="form.password" type="button" @click="copyPassword" class="absolute right-2 top-1/2 -translate-y-1/2 rounded-lg p-1 transition-colors hover:bg-gray-100 dark:hover:bg-dark-700" :class="passwordCopied ? 'text-green-500' : 'text-gray-400'">
              <svg v-if="passwordCopied" class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" /></svg>
              <svg v-else class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M15.666 3.888A2.25 2.25 0 0013.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 01-.75.75H9a.75.75 0 01-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 01-2.25 2.25H6.75A2.25 2.25 0 014.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 011.927-.184" /></svg>
            </button>
          </div>
          <button type="button" @click="generatePassword" class="btn btn-secondary px-3">
            <Icon name="refresh" size="md" />
          </button>
        </div>
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.username') }}</label>
        <input v-model="form.username" type="text" class="input" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.form.roleLabel') }}</label>
        <Select
          v-model="form.role"
          :options="roleOptions"
          :searchable="false"
        />
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.notes') }}</label>
        <textarea v-model="form.notes" rows="3" class="input"></textarea>
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.columns.concurrency') }}</label>
        <input
          v-model.number="form.concurrency"
          type="number"
          min="0"
          step="1"
          class="input"
          :placeholder="t('admin.users.form.concurrencyPlaceholder')"
          data-test="concurrency-input"
        />
        <p class="input-hint">{{ t('admin.users.form.concurrencyHint') }}</p>
      </div>
      <div>
        <label class="input-label">{{ t('admin.users.form.rpmLimit') }}</label>
        <input
          v-model.number="form.rpm_limit"
          type="number"
          min="0"
          step="1"
          class="input"
          :placeholder="t('admin.users.form.rpmLimitPlaceholder')"
        />
        <p class="input-hint">{{ t('admin.users.form.rpmLimitHint') }}</p>
      </div>
      <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
        <h4 class="font-medium text-gray-900 dark:text-white">{{ t('admin.users.form.tokenLimitsTitle') }}</h4>
        <p class="input-hint mb-3">{{ t('admin.users.form.tokenLimitsHint') }}</p>
        <div class="grid gap-3 sm:grid-cols-3">
          <div v-for="window in tokenLimitWindows" :key="window.key">
            <label class="input-label">{{ t(window.label) }}</label>
            <input
              v-model.number="form[window.key]"
              type="number"
              min="0"
              step="1"
              class="input"
              :placeholder="t('admin.users.form.tokenLimitPlaceholder')"
            />
          </div>
        </div>
      </div>
      <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
        <div class="flex items-start justify-between gap-3">
          <div>
            <h4 class="font-medium text-gray-900 dark:text-white">{{ t('admin.users.form.modelRestrictionsTitle') }}</h4>
            <p class="input-hint">{{ t('admin.users.form.modelRestrictionsHint') }}</p>
          </div>
          <button type="button" class="btn btn-secondary shrink-0" @click="addModelRestriction">
            {{ t('admin.users.form.addModelRestriction') }}
          </button>
        </div>
        <div v-if="form.model_restrictions.length === 0" class="mt-3 text-sm text-gray-500 dark:text-dark-400">
          {{ t('admin.users.form.noModelRestrictions') }}
        </div>
        <div
          v-for="(rule, index) in form.model_restrictions"
          :key="index"
          class="mt-3 rounded-lg bg-gray-50 p-3 dark:bg-dark-800"
        >
          <div class="flex gap-2">
            <input
              v-model="rule.model_pattern"
              type="text"
              class="input flex-1"
              :placeholder="t('admin.users.form.modelPatternPlaceholder')"
            />
            <button type="button" class="btn btn-secondary" @click="removeModelRestriction(index)">
              {{ t('common.delete') }}
            </button>
          </div>
          <div class="mt-3 flex flex-wrap gap-x-4 gap-y-2">
            <label v-for="effort in reasoningEffortOptions" :key="effort" class="flex items-center gap-1.5 text-sm">
              <input v-model="rule.reasoning_efforts" type="checkbox" :value="effort" class="rounded" />
              <span>{{ effort }}</span>
            </label>
          </div>
          <p class="input-hint">{{ t('admin.users.form.reasoningEffortsHint') }}</p>
        </div>
      </div>
      <UserAttributeForm v-model="form.customAttributes" :user-id="user?.id" />
    </form>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button @click="$emit('close')" type="button" class="btn btn-secondary">{{ t('common.cancel') }}</button>
        <button type="submit" form="edit-user-form" :disabled="submitting" class="btn btn-primary">
          {{ submitting ? t('admin.users.updating') : t('common.update') }}
        </button>
      </div>
    </template>
  </BaseDialog>

  <!-- 角色提升为管理员时后端要求 step-up 2FA，弹出 TOTP 验证后自动重试 -->
  <TotpStepUpDialog :controller="stepUp" />
</template>

<script setup lang="ts">
import { computed, ref, reactive, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'
import { adminAPI } from '@/api/admin'
import type { AdminUser, UpdateUserRequest, UserAttributeValuesMap, UserModelRestriction } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import UserAttributeForm from '@/components/user/UserAttributeForm.vue'
import Icon from '@/components/icons/Icon.vue'
import { useStepUp, isStepUpBlocked, isStepUpCancelled, stepUpBlockReason } from '@/composables/useStepUp'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'

const props = defineProps<{ show: boolean, user: AdminUser | null }>()
const emit = defineEmits(['close', 'success'])
const { t } = useI18n(); const appStore = useAppStore(); const { copyToClipboard } = useClipboard()

const submitting = ref(false); const passwordCopied = ref(false)
const roleOptions = computed(() => [
  { value: 'user', label: t('admin.users.roles.user') },
  { value: 'admin', label: t('admin.users.roles.admin') }
])
const reasoningEffortOptions = ['minimal', 'low', 'medium', 'high', 'xhigh', 'max'] as const
const tokenLimitWindows = [
  { key: 'token_limit_1d', label: 'admin.users.form.tokenLimit1d' },
  { key: 'token_limit_7d', label: 'admin.users.form.tokenLimit7d' },
  { key: 'token_limit_30d', label: 'admin.users.form.tokenLimit30d' },
] as const
const form = reactive({
  email: '', password: '', username: '', notes: '', role: 'user' as AdminUser['role'], concurrency: 1, rpm_limit: 0,
  token_limit_1d: 0, token_limit_7d: 0, token_limit_30d: 0,
  model_restrictions: [] as UserModelRestriction[], customAttributes: {} as UserAttributeValuesMap,
})

watch(() => props.user, (u) => {
  if (u) {
    Object.assign(form, {
      email: u.email, password: '', username: u.username || '', notes: u.notes || '', role: u.role || 'user',
      concurrency: u.concurrency, rpm_limit: u.rpm_limit ?? 0,
      token_limit_1d: u.token_limit_1d ?? 0, token_limit_7d: u.token_limit_7d ?? 0, token_limit_30d: u.token_limit_30d ?? 0,
      model_restrictions: (u.model_restrictions ?? []).map((rule) => ({
        model_pattern: rule.model_pattern,
        reasoning_efforts: [...(rule.reasoning_efforts ?? [])],
      })),
      customAttributes: {},
    })
    passwordCopied.value = false
  }
}, { immediate: true })

const generatePassword = () => {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789!@#$%^&*'
  let p = ''; for (let i = 0; i < 16; i++) p += chars.charAt(Math.floor(Math.random() * chars.length))
  form.password = p
}
const copyPassword = async () => {
  if (form.password && await copyToClipboard(form.password, t('admin.users.passwordCopied'))) {
    passwordCopied.value = true; setTimeout(() => passwordCopied.value = false, 2000)
  }
}
const stepUp = useStepUp()
const addModelRestriction = () => form.model_restrictions.push({ model_pattern: '', reasoning_efforts: [] })
const removeModelRestriction = (index: number) => form.model_restrictions.splice(index, 1)

const handleUpdateUser = async () => {
  if (!props.user) return
  if (!form.email.trim()) {
    appStore.showError(t('admin.users.emailRequired'))
    return
  }
  // 0 = 不限制，与网关 (AcquireUserSlot: maxConcurrency <= 0) 和批量改限额一致
  if (!Number.isInteger(form.concurrency) || form.concurrency < 0) {
    appStore.showError(t('admin.users.concurrencyNonNegative'))
    return
  }
  const tokenLimits = [form.token_limit_1d, form.token_limit_7d, form.token_limit_30d]
  if (tokenLimits.some((value) => !Number.isSafeInteger(value) || value < 0)) {
    appStore.showError(t('admin.users.form.tokenLimitInvalid'))
    return
  }
  if (form.model_restrictions.some((rule) => !rule.model_pattern.trim())) {
    appStore.showError(t('admin.users.form.modelPatternRequired'))
    return
  }
  const userId = props.user.id
  submitting.value = true
  try {
    const data: UpdateUserRequest = {
      email: form.email, username: form.username, notes: form.notes, role: form.role,
      concurrency: form.concurrency, rpm_limit: form.rpm_limit,
      token_limit_1d: form.token_limit_1d, token_limit_7d: form.token_limit_7d, token_limit_30d: form.token_limit_30d,
      model_restrictions: form.model_restrictions.map((rule) => ({
        model_pattern: rule.model_pattern.trim(),
        reasoning_efforts: [...rule.reasoning_efforts],
      })),
    }
    if (form.password.trim()) data.password = form.password.trim()
    // 提升为管理员属敏感操作：后端返回 STEP_UP_REQUIRED 时弹 TOTP 验证并重试
    await stepUp.run(() => adminAPI.users.update(userId, data))
    if (Object.keys(form.customAttributes).length > 0) await adminAPI.userAttributes.updateUserAttributeValues(userId, form.customAttributes)
    appStore.showSuccess(t('admin.users.userUpdated'))
    emit('success'); emit('close')
  } catch (e: any) {
    if (isStepUpCancelled(e)) {
      // 用户主动取消二次验证：静默返回，表单保持打开。
    } else if (isStepUpBlocked(e)) {
      appStore.showError(
        stepUpBlockReason(e) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN'
          ? t('stepUp.adminApiKeyForbidden')
          : t('stepUp.notEnabled')
      )
    } else {
      appStore.showError(e?.message || t('admin.users.failedToUpdate'))
    }
  } finally { submitting.value = false }
}
</script>
