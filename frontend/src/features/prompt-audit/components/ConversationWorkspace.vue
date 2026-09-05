<template>
  <section class="py-6">
    <div class="flex flex-wrap items-end justify-between gap-3">
      <div>
        <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.promptAudit.conversations.title') }}</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('admin.promptAudit.conversations.description') }}</p>
      </div>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="load">{{ t('admin.promptAudit.actions.refresh') }}</button>
    </div>

    <form class="mt-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-5" @submit.prevent="search">
      <select v-model="filters.request_kind" class="input" :aria-label="t('admin.promptAudit.conversations.requestKind')">
        <option value="conversation">{{ t('admin.promptAudit.conversations.kinds.conversation') }}</option>
        <option value="auxiliary">{{ t('admin.promptAudit.conversations.kinds.auxiliary') }}</option>
        <option value="unknown">{{ t('admin.promptAudit.conversations.kinds.unknown') }}</option>
        <option value="">{{ t('common.all') }}</option>
      </select>
      <input v-model="filters.keyword" class="input" type="search" :placeholder="t('admin.promptAudit.conversations.keyword')" />
      <input v-model="filters.conversation_id" class="input" :placeholder="t('admin.promptAudit.conversations.conversationId')" />
      <input v-model="filters.request_id" class="input" :placeholder="t('admin.promptAudit.events.requestId')" />
      <select v-model="filters.review_status" class="input">
        <option value="">{{ t('admin.promptAudit.conversations.allStatuses') }}</option>
        <option value="pending">pending</option><option value="processing">processing</option><option value="reviewed">reviewed</option><option value="failed">failed</option>
      </select>
      <select v-model="filters.decision" class="input">
        <option value="">{{ t('admin.promptAudit.conversations.allDecisions') }}</option>
        <option value="pass">{{ t('admin.promptAudit.decisions.pass') }}</option><option value="flag">{{ t('admin.promptAudit.decisions.flag') }}</option><option value="critical">{{ t('admin.promptAudit.decisions.critical') }}</option>
      </select>
      <input v-model="filters.group_id" class="input" inputmode="numeric" :placeholder="t('admin.promptAudit.events.groupId')" />
      <input v-model="filters.user_id" class="input" inputmode="numeric" :placeholder="t('admin.promptAudit.events.userId')" />
      <input v-model="filters.api_key_id" class="input" inputmode="numeric" :placeholder="t('admin.promptAudit.events.apiKeyId')" />
      <button type="submit" class="btn btn-primary">{{ t('common.search') }}</button>
    </form>

    <div v-if="error" class="mt-4 rounded-lg bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ error }}</div>
    <div class="mt-5 overflow-x-auto border-y border-gray-200 dark:border-dark-700">
      <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
        <thead class="bg-gray-50 text-left text-xs uppercase text-gray-500 dark:bg-dark-900/40 dark:text-dark-400">
          <tr><th class="px-3 py-3">{{ t('admin.promptAudit.conversations.lastTurn') }}</th><th class="px-3 py-3">{{ t('admin.promptAudit.events.identity') }}</th><th class="px-3 py-3">{{ t('admin.promptAudit.events.route') }}</th><th class="px-3 py-3">{{ t('admin.promptAudit.conversations.reviewState') }}</th><th class="px-3 py-3 text-right">{{ t('admin.promptAudit.common.actions') }}</th></tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
          <tr v-for="session in page.items" :key="session.id">
            <td class="whitespace-nowrap px-3 py-3 text-gray-700 dark:text-dark-200">{{ formatDate(session.last_turn_at) }}</td>
            <td class="px-3 py-3"><div class="font-medium text-gray-900 dark:text-white">{{ session.username || `#${session.user_id}` }}</div><div class="text-xs text-gray-500">{{ session.group_name || `Group #${session.group_id || '-'}` }}</div></td>
            <td class="max-w-sm px-3 py-3"><div v-if="session.user_preview" class="mb-1 line-clamp-2 break-words text-sm text-gray-900 dark:text-white">{{ session.user_preview }}</div><div class="text-gray-800 dark:text-dark-100">{{ session.model }}</div><div class="text-xs text-gray-500">{{ session.protocol }} · {{ session.turn_count }} turns</div></td>
            <td class="px-3 py-3"><div>{{ session.latest_review_decision || 'pending' }}</div><div class="text-xs text-gray-500">{{ session.flagged_turn_count }} flagged · {{ session.pending_review_count }} pending</div></td>
            <td class="whitespace-nowrap px-3 py-3 text-right"><button class="btn btn-secondary btn-sm mr-2" @click="open(session.id)">{{ t('common.view') }}</button><button class="btn btn-danger btn-sm" @click="remove(session.id)">{{ t('common.delete') }}</button></td>
          </tr>
          <tr v-if="!loading && page.items.length === 0"><td colspan="5" class="px-3 py-10 text-center text-gray-500">{{ t('admin.promptAudit.conversations.empty') }}</td></tr>
        </tbody>
      </table>
    </div>
    <div class="mt-4 flex items-center justify-between text-sm text-gray-500">
      <span>{{ page.total }} {{ t('admin.promptAudit.conversations.sessions') }}</span>
      <div class="flex gap-2"><button class="btn btn-secondary btn-sm" :disabled="page.page <= 1 || loading" @click="changePage(page.page - 1)">{{ t('admin.promptAudit.conversations.previous') }}</button><button class="btn btn-secondary btn-sm" :disabled="page.page >= page.pages || loading" @click="changePage(page.page + 1)">{{ t('admin.promptAudit.conversations.next') }}</button></div>
    </div>

    <div v-if="detailOpen" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" @click.self="close">
      <div class="max-h-[90vh] w-full max-w-5xl overflow-y-auto rounded-lg bg-white p-5 shadow-xl dark:bg-dark-900">
        <div class="flex items-center justify-between"><h3 class="text-lg font-semibold text-gray-950 dark:text-white">{{ t('admin.promptAudit.conversations.detailTitle') }}</h3><button class="btn btn-secondary btn-sm" @click="close">{{ t('common.close') }}</button></div>
        <div v-if="detailLoading" class="py-12 text-center text-gray-500">{{ t('common.loading') }}</div>
        <div v-else-if="detail" class="mt-5 space-y-6">
          <div class="flex flex-wrap gap-2" role="tablist" :aria-label="t('admin.promptAudit.conversations.viewMode')">
            <button type="button" role="tab" :aria-selected="viewMode === 'dialogue'" class="border-b-2 px-3 py-2 text-sm" :class="viewMode === 'dialogue' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500'" @click="viewMode = 'dialogue'">{{ t('admin.promptAudit.conversations.fullConversation') }}</button>
            <button type="button" role="tab" :aria-selected="viewMode === 'requests'" class="border-b-2 px-3 py-2 text-sm" :class="viewMode === 'requests' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500'" @click="viewMode = 'requests'">{{ t('admin.promptAudit.conversations.perRequest') }}</button>
          </div>
          <div class="grid gap-3 text-sm sm:grid-cols-3"><div><span class="text-gray-500">ID</span><p>{{ detail.external_conversation_id || `#${detail.id}` }}</p></div><div><span class="text-gray-500">{{ t('admin.promptAudit.events.identity') }}</span><p>{{ detail.username }} · {{ detail.api_key_name }}</p></div><div><span class="text-gray-500">{{ t('admin.promptAudit.events.route') }}</span><p>{{ detail.protocol }} · {{ detail.model }}</p></div></div>
          <template v-if="viewMode === 'dialogue'">
            <p v-if="!timeline.length" class="text-sm text-gray-500">{{ t('admin.promptAudit.conversations.onlyAuxiliary') }}</p>
            <ConversationTurnDetail v-for="entry in timeline" :key="entry.turn.id" :turn="entry.turn" :timeline-messages="entry.messages" />
          </template>
          <template v-else><ConversationTurnDetail v-for="turn in detail.turns" :key="turn.id" :turn="turn" /></template>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import promptAuditAPI from '../api'
import ConversationTurnDetail from './ConversationTurnDetail.vue'
import { buildConversationTimeline } from '../conversationTimeline'
import type { ConversationFilters, ConversationPage, ConversationSession } from '../types'
import { cloneData, emptyConversationFilters } from '../viewModel'

const { t, locale } = useI18n()
const appStore = useAppStore()
const filters = reactive<ConversationFilters>(emptyConversationFilters())
const applied = ref<ConversationFilters>(emptyConversationFilters())
const page = reactive<ConversationPage>({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
const loading = ref(false)
const error = ref('')
const detailOpen = ref(false)
const detailLoading = ref(false)
const detail = ref<ConversationSession | null>(null)
const viewMode = ref<'dialogue' | 'requests'>('dialogue')
const timeline = computed(() => buildConversationTimeline(detail.value?.turns || []))

async function load() {
  loading.value = true; error.value = ''
  try { Object.assign(page, await promptAuditAPI.listConversations(applied.value, page.page, page.page_size)) }
  catch { error.value = t('admin.promptAudit.errors.loadConversations') }
  finally { loading.value = false }
}
function search() { applied.value = cloneData(filters); page.page = 1; void load() }
function changePage(value: number) { page.page = value; void load() }
async function open(id: number) { detailOpen.value = true; detailLoading.value = true; viewMode.value = 'dialogue'; try { detail.value = await promptAuditAPI.getConversation(id); if (detail.value.turns?.every(turn => turn.request_kind === 'auxiliary')) viewMode.value = 'requests' } catch { close(); appStore.showError(t('admin.promptAudit.errors.loadConversationDetail')) } finally { detailLoading.value = false } }
function close() { detailOpen.value = false; detail.value = null }
async function remove(id: number) { if (!window.confirm(t('admin.promptAudit.conversations.deleteConfirm'))) return; try { await promptAuditAPI.deleteConversation(id); appStore.showSuccess(t('admin.promptAudit.conversations.deleted')); await load() } catch { appStore.showError(t('admin.promptAudit.errors.deleteConversation')) } }
function formatDate(value: string) { return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) }
onMounted(load)
</script>
