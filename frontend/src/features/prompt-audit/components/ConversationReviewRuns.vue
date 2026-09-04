<template>
  <section class="py-6">
    <div class="flex flex-wrap items-end justify-between gap-3"><div><h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.promptAudit.reviewRuns.title') }}</h2><p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('admin.promptAudit.reviewRuns.description') }}</p></div><div class="flex gap-2"><button class="btn btn-secondary btn-sm" :disabled="loading" @click="load">{{ t('admin.promptAudit.actions.refresh') }}</button><button class="btn btn-primary btn-sm" :disabled="running || !enabled" @click="runNow">{{ running ? t('admin.promptAudit.reviewRuns.queueing') : t('admin.promptAudit.reviewRuns.runNow') }}</button></div></div>
    <p v-if="!enabled" class="mt-4 rounded-lg bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:bg-amber-950/30 dark:text-amber-200">{{ t('admin.promptAudit.reviewRuns.disabledHint') }}</p>
    <div class="mt-5 overflow-x-auto border-y border-gray-200 dark:border-dark-700"><table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700"><thead class="bg-gray-50 text-left text-xs uppercase text-gray-500 dark:bg-dark-900/40"><tr><th class="px-3 py-3">{{ t('admin.promptAudit.reviewRuns.created') }}</th><th class="px-3 py-3">{{ t('admin.promptAudit.reviewRuns.trigger') }}</th><th class="px-3 py-3">{{ t('admin.promptAudit.reviewRuns.status') }}</th><th class="px-3 py-3">{{ t('admin.promptAudit.reviewRuns.result') }}</th><th class="px-3 py-3">{{ t('admin.promptAudit.reviewRuns.error') }}</th></tr></thead><tbody class="divide-y divide-gray-100 dark:divide-dark-800"><tr v-for="run in page.items" :key="run.id"><td class="px-3 py-3">{{ formatDate(run.created_at) }}</td><td class="px-3 py-3">{{ run.trigger_type }}</td><td class="px-3 py-3">{{ run.status }}</td><td class="px-3 py-3">{{ run.processed_count }} processed · {{ run.flagged_count }} flagged · {{ run.failed_count }} failed</td><td class="px-3 py-3 text-xs text-red-600">{{ run.last_error_code }}</td></tr><tr v-if="!loading && !page.items.length"><td colspan="5" class="px-3 py-10 text-center text-gray-500">{{ t('admin.promptAudit.reviewRuns.empty') }}</td></tr></tbody></table></div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import promptAuditAPI from '../api'
import type { ConversationReviewRunPage } from '../types'

defineProps<{ enabled: boolean }>()
const { t, locale } = useI18n()
const appStore = useAppStore()
const page = reactive<ConversationReviewRunPage>({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
const loading = ref(false)
const running = ref(false)
async function load() { loading.value = true; try { Object.assign(page, await promptAuditAPI.listConversationReviewRuns(page.page, page.page_size)) } catch { appStore.showError(t('admin.promptAudit.errors.loadReviewRuns')) } finally { loading.value = false } }
async function runNow() { running.value = true; try { await promptAuditAPI.runConversationReview(); appStore.showSuccess(t('admin.promptAudit.reviewRuns.queued')); await load() } catch { appStore.showError(t('admin.promptAudit.errors.runReview')) } finally { running.value = false } }
function formatDate(value: string) { return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) }
onMounted(load)
</script>
