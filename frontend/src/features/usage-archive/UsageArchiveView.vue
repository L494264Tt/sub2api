<template>
  <div class="space-y-6 p-6">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="text-xl font-semibold text-gray-950 dark:text-white">{{ t('admin.users.usageArchive.title') }}</h1>
        <p class="mt-1 text-sm text-gray-500">{{ t('admin.users.usageArchive.description') }}</p>
      </div>
      <button class="btn btn-secondary" :disabled="loading" @click="loadAll">{{ t('common.refresh') }}</button>
    </div>

    <div class="grid gap-3 sm:grid-cols-4">
      <div v-for="item in statItems" :key="item.label" class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
        <div class="text-xs text-gray-500">{{ item.label }}</div>
        <div class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ item.value }}</div>
      </div>
    </div>

    <form class="grid gap-3 rounded-lg border border-gray-200 p-4 sm:grid-cols-4 dark:border-dark-700" @submit.prevent="search">
      <input v-model="filters.user_id" class="input" :placeholder="t('admin.users.usageArchive.userId')" />
      <input v-model="filters.api_key_id" class="input" :placeholder="t('admin.users.usageArchive.apiKeyId')" />
      <input v-model="filters.model" class="input" :placeholder="t('admin.users.usageArchive.modelPlaceholder')" />
      <input v-model="filters.request_id" class="input" :placeholder="t('admin.users.usageArchive.requestId')" />
      <div class="flex gap-2 sm:col-span-4">
        <button class="btn btn-primary" type="submit">{{ t('common.search') }}</button>
        <button class="btn btn-secondary" type="button" @click="resetFilters">{{ t('common.reset') }}</button>
      </div>
    </form>

    <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
      <table class="min-w-full text-left text-sm">
        <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-900/40">
          <tr><th v-for="label in columns" :key="label" class="px-3 py-3">{{ label }}</th></tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
          <tr v-for="session in page.items" :key="session.id" class="cursor-pointer hover:bg-gray-50 dark:hover:bg-dark-800" @click="openSession(session.id)">
            <td class="px-3 py-3">{{ formatDate(session.last_seen_at) }}</td>
            <td class="px-3 py-3">#{{ session.user_id }} / #{{ session.api_key_id }}</td>
            <td class="px-3 py-3">{{ session.provider }} · {{ session.model }}</td>
            <td class="px-3 py-3">{{ session.turn_count }}</td>
            <td class="px-3 py-3">{{ formatBytes(session.content_bytes) }}</td>
          </tr>
          <tr v-if="!loading && !page.items.length"><td colspan="5" class="px-3 py-10 text-center text-gray-500">{{ t('admin.users.usageArchive.empty') }}</td></tr>
        </tbody>
      </table>
    </div>

    <div class="flex items-center justify-between text-sm text-gray-500">
      <span>{{ page.total }} {{ t('admin.users.usageArchive.sessions') }}</span>
      <div class="flex gap-2">
        <button class="btn btn-secondary btn-sm" :disabled="page.page <= 1 || loading" @click="changePage(page.page - 1)">{{ t('pagination.previous') }}</button>
        <button class="btn btn-secondary btn-sm" :disabled="page.page >= page.pages || loading" @click="changePage(page.page + 1)">{{ t('pagination.next') }}</button>
      </div>
    </div>

    <div v-if="detail" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" @click.self="detail = null">
      <div class="max-h-[90vh] w-full max-w-6xl overflow-y-auto rounded-lg bg-white p-5 shadow-xl dark:bg-dark-900">
        <div class="flex items-center justify-between gap-3"><h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.users.usageArchive.detail') }}</h2><button class="btn btn-secondary btn-sm" @click="detail = null">{{ t('common.close') }}</button></div>
        <div class="mt-4 space-y-5">
          <div v-for="turn in detail.turns" :key="turn.id" class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
            <div class="flex flex-wrap justify-between gap-2 text-xs text-gray-500"><span>#{{ turn.sequence_no }} · {{ turn.protocol }} · {{ turn.model }}</span><span>{{ formatDate(turn.captured_at) }} · {{ turn.status_code }}</span></div>
            <div class="mt-3 space-y-3">
              <div v-for="message in turn.messages" :key="message.id" class="rounded border border-gray-100 p-3 dark:border-dark-800">
                <div class="text-xs font-semibold uppercase text-primary-600">{{ message.role }} · {{ message.source }}</div>
                <pre v-if="message.content" class="mt-2 whitespace-pre-wrap break-words text-sm text-gray-800 dark:text-gray-100">{{ message.content }}</pre>
                <div v-else class="mt-2 text-xs text-gray-500">{{ message.content_expired ? t('admin.users.usageArchive.contentExpired') : t('admin.users.usageArchive.metadataOnly') }}</div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import usageArchiveAPI from './api'
import type { ArchiveFilters, ArchiveSession, ArchiveSessionPage, ArchiveStats } from './types'

const { t, locale } = useI18n()
const filters = reactive<ArchiveFilters>({ user_id: '', api_key_id: '', group_id: '', provider: '', model: '', request_id: '', session_key: '' })
const page = reactive<ArchiveSessionPage>({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
const stats = ref<ArchiveStats>({ sessions: 0, turns: 0, messages: 0, content_bytes: 0, user_messages: 0, prompt_messages: 0, assistant_messages: 0 })
const detail = ref<ArchiveSession | null>(null)
const loading = ref(false)
const columns = computed(() => [t('admin.users.usageArchive.time'), t('admin.users.usageArchive.identity'), t('admin.users.usageArchive.model'), t('admin.users.usageArchive.turns'), t('admin.users.usageArchive.storage')])
const statItems = computed(() => [
  { label: t('admin.users.usageArchive.sessions'), value: formatNumber(stats.value.sessions) },
  { label: t('admin.users.usageArchive.turns'), value: formatNumber(stats.value.turns) },
  { label: t('admin.users.usageArchive.messages'), value: formatNumber(stats.value.messages) },
  { label: t('admin.users.usageArchive.contentBytes'), value: formatBytes(stats.value.content_bytes) },
])

async function loadAll() {
  loading.value = true
  try {
    const [list, currentStats] = await Promise.all([usageArchiveAPI.listSessions(filters, page.page, page.page_size), usageArchiveAPI.getStats()])
    Object.assign(page, list); stats.value = currentStats
  } finally { loading.value = false }
}
function search() { page.page = 1; void loadAll() }
function resetFilters() { Object.keys(filters).forEach((key) => { filters[key as keyof ArchiveFilters] = '' }); search() }
function changePage(value: number) { page.page = value; void loadAll() }
async function openSession(id: number) { detail.value = await usageArchiveAPI.getSession(id) }
function formatNumber(value: number) { return new Intl.NumberFormat(locale.value).format(value) }
function formatBytes(value: number) { if (value < 1024) return `${value} B`; if (value < 1024 ** 2) return `${(value / 1024).toFixed(1)} KB`; if (value < 1024 ** 3) return `${(value / 1024 ** 2).toFixed(1)} MB`; return `${(value / 1024 ** 3).toFixed(2)} GB` }
function formatDate(value: string) { return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) }
onMounted(() => { void loadAll() })
</script>
