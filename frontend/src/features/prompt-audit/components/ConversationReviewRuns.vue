<template>
  <section class="py-6" :aria-busy="loading">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.promptAudit.reviewRuns.title') }}</h2>
      <div class="flex gap-2">
        <button type="button" class="btn btn-secondary btn-sm" :title="t('admin.promptAudit.actions.refresh')" :aria-label="t('admin.promptAudit.actions.refresh')" :disabled="loading || running" data-test="refresh-runs" @click="load()">
          <Icon name="refresh" size="sm" />
        </button>
        <button type="button" class="btn btn-primary btn-sm" :disabled="running || loading || !enabled" data-test="run-review" @click="runNow">
          <Icon name="play" size="sm" class="mr-1.5" />
          {{ running ? t('admin.promptAudit.reviewRuns.queueing') : t('admin.promptAudit.reviewRuns.runNow') }}
        </button>
      </div>
    </div>
    <p v-if="!enabled" class="mt-4 rounded-lg bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:bg-amber-950/30 dark:text-amber-200">{{ t('admin.promptAudit.reviewRuns.disabledHint') }}</p>
    <p v-if="error" role="alert" class="mt-4 text-sm text-red-600">{{ error }}</p>
    <div class="mt-5 overflow-x-auto border-y border-gray-200 dark:border-dark-700">
      <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
        <thead class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-900/40">
          <tr><th v-for="column in columns" :key="column" class="whitespace-nowrap px-3 py-3">{{ t(`admin.promptAudit.reviewRuns.${column}`) }}</th></tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
          <tr v-for="run in page.items" :key="run.id" :data-run-id="run.id">
            <td class="whitespace-nowrap px-3 py-3">{{ formatDate(run.created_at) }}</td>
            <td class="whitespace-nowrap px-3 py-3">{{ triggerLabel(run.trigger_type) }}</td>
            <td class="whitespace-nowrap px-3 py-3">{{ statusLabel(run.status) }}</td>
            <td class="px-3 py-3">{{ t('admin.promptAudit.reviewRuns.counts', { processed: number(run.processed_count), flagged: number(run.flagged_count), failed: number(run.failed_count) }) }}</td>
            <td class="max-w-xs break-words px-3 py-3 text-xs text-red-600">{{ run.last_error_message || run.last_error_code || t('admin.promptAudit.reviewRuns.noError') }}</td>
          </tr>
          <tr v-if="loading && !page.items.length"><td colspan="5" class="px-3 py-10 text-center text-gray-500">{{ t('common.loading') }}</td></tr>
          <tr v-else-if="!page.items.length && !error"><td colspan="5" class="px-3 py-10 text-center text-gray-500">{{ t('admin.promptAudit.reviewRuns.empty') }}</td></tr>
        </tbody>
      </table>
    </div>
    <fieldset :disabled="loading || running" class="min-w-0" :class="{ 'pointer-events-none opacity-60': loading || running }">
      <Pagination :total="page.total" :page="page.page" :page-size="page.page_size" :page-size-options="pageSizes" @update:page="changePage" @update:page-size="changePageSize" />
    </fieldset>
  </section>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useAppStore } from '@/stores/app'
import promptAuditAPI from '../api'
import type { ConversationReviewRunPage } from '../types'

const props = defineProps<{ enabled: boolean }>()
const { t, locale } = useI18n()
const appStore = useAppStore()
const DEFAULT_PAGE_SIZE = 20
const REFRESH_INTERVAL_MS = 5000
const pageSizes = [20, 50, 100]
const columns = ['created', 'trigger', 'status', 'result', 'error'] as const
const page = reactive<ConversationReviewRunPage>({ items: [], total: 0, page: 1, page_size: DEFAULT_PAGE_SIZE, pages: 0 })
const loading = ref(false)
const running = ref(false)
const error = ref('')
let disposed = false
let refreshTimer: ReturnType<typeof setInterval> | undefined

async function load(targetPage = page.page, pageSize = page.page_size) {
  if (loading.value || disposed) return
  loading.value = true
  try {
    const result = await promptAuditAPI.listConversationReviewRuns(targetPage, pageSize)
    if (disposed) return
    Object.assign(page, result)
    error.value = ''
  } catch {
    if (!disposed) error.value = t('admin.promptAudit.errors.loadReviewRuns')
  } finally {
    if (!disposed) loading.value = false
  }
}

function changePage(value: number) {
  if (!running.value) void load(value)
}

function changePageSize(value: number) {
  if (!running.value) void load(1, value)
}

async function runNow() {
  if (running.value || loading.value || !props.enabled || disposed) return
  running.value = true
  try {
    await promptAuditAPI.runConversationReview()
    if (disposed) return
    appStore.showSuccess(t('admin.promptAudit.reviewRuns.queued'))
    await load(1)
  } catch {
    if (!disposed) appStore.showError(t('admin.promptAudit.errors.runReview'))
  } finally {
    if (!disposed) running.value = false
  }
}

function statusLabel(value: string) {
  return ['queued', 'processing', 'completed', 'failed'].includes(value)
    ? t(`admin.promptAudit.reviewRuns.statuses.${value}`) : value
}
function triggerLabel(value: string) {
  return ['scheduled', 'manual'].includes(value) ? t(`admin.promptAudit.reviewRuns.triggers.${value}`) : value
}
function number(value: number) { return new Intl.NumberFormat(locale.value).format(value) }
function formatDate(value: string) { return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) }

onMounted(() => {
  void load()
  refreshTimer = setInterval(() => {
    if (!document.hidden && !running.value) void load()
  }, REFRESH_INTERVAL_MS)
})
onUnmounted(() => {
  disposed = true
  clearInterval(refreshTimer)
})
</script>
