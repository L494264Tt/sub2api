<template>
  <article class="min-w-0 border-t border-gray-200 pt-5 dark:border-dark-700">
    <div class="flex flex-wrap justify-between gap-2 text-xs text-gray-500 [overflow-wrap:anywhere]">
      <span>{{ date }} · {{ turn.request_id }}</span>
      <span>{{ t(`admin.promptAudit.conversations.kinds.${turn.request_kind || 'unknown'}`) }}</span>
    </div>
    <p v-if="turn.request_truncated || turn.response_truncated" role="status" class="mt-3 text-sm text-amber-700 dark:text-amber-300">{{ t('admin.promptAudit.conversations.truncated') }}</p>
    <p v-if="!structured" class="mt-3 text-sm text-amber-700 dark:text-amber-300" data-test="legacy-note">{{ t('admin.promptAudit.conversations.legacyNote') }}</p>
    <p v-if="auxiliary" class="mt-3 text-sm text-amber-700 dark:text-amber-300">{{ t(isOpenClawHeartbeat(turn) ? 'admin.promptAudit.openClaw.heartbeatMeaning' : 'admin.promptAudit.conversations.auxiliaryNote') }}</p>
    <p v-if="turn.request_details?.association === 'history_match'" class="mt-2 text-xs text-gray-500">{{ t('admin.promptAudit.conversations.historyMatched') }}</p>
    <p v-else-if="turn.request_details?.association === 'request_id'" class="mt-2 text-xs text-gray-500">{{ t('admin.promptAudit.conversations.unlinked') }}</p>

    <div v-if="structured && !auxiliary" class="mt-4 space-y-4" data-test="current-messages">
      <ConversationMessage v-for="message in current" :key="message.position" :message="message" />
      <p v-if="!current.length" class="text-sm text-gray-500">{{ t('admin.promptAudit.conversations.noUserMessage') }}</p>
    </div>
    <div class="mt-4" data-test="model-response">
      <ConversationMessage :message="{ role: 'assistant', kind: 'text', content: turn.model_response || t('admin.promptAudit.conversations.noResponse'), position: -1, truncated: turn.response_truncated }" />
    </div>

    <details v-if="structured && (history.length || auxiliary)" class="mt-5 border-t border-gray-100 pt-3 dark:border-dark-800" data-test="history">
      <summary class="cursor-pointer text-sm font-medium text-gray-600 dark:text-dark-200">{{ t(auxiliary ? 'admin.promptAudit.conversations.auxiliaryInput' : 'admin.promptAudit.conversations.history') }}</summary>
      <div class="mt-4 space-y-4"><ConversationMessage v-for="message in auxiliary ? messages : history" :key="message.position" :message="message" /></div>
    </details>
    <p v-if="turn.request_details?.omitted_messages" class="mt-2 text-xs text-amber-700">{{ t('admin.promptAudit.conversations.omittedMessages', { count: turn.request_details.omitted_messages }) }}</p>

    <details v-if="structured && (context.length || turn.request_details?.context_truncated)" class="mt-4 border-t border-gray-100 pt-3 dark:border-dark-800" data-test="context">
      <summary class="cursor-pointer text-sm font-medium text-gray-600 dark:text-dark-200">{{ t('admin.promptAudit.conversations.context') }}</summary>
      <p v-if="turn.request_details?.context_truncated" class="mt-2 text-xs text-amber-700 dark:text-amber-300">{{ t('admin.promptAudit.conversations.contextTruncated') }}</p>
      <div class="mt-4 space-y-4"><ConversationMessage v-for="message in context" :key="message.position" :message="message" /></div>
    </details>

    <details class="mt-4 border-t border-gray-100 pt-3 dark:border-dark-800" data-test="saved-request">
      <summary class="cursor-pointer text-sm font-medium text-gray-600 dark:text-dark-200">{{ t('admin.promptAudit.conversations.savedRequest') }}</summary>
      <pre class="mt-3 max-h-96 overflow-auto whitespace-pre-wrap break-words text-xs text-gray-700 dark:text-dark-200 [overflow-wrap:anywhere]">{{ structured ? JSON.stringify(turn.request_details, null, 2) : turn.request_transcript }}</pre>
    </details>
    <details v-if="turn.request_details?.response_context?.length" class="mt-4 border-t border-gray-100 pt-3 dark:border-dark-800" data-test="response-context">
      <summary class="cursor-pointer text-sm font-medium text-gray-600 dark:text-dark-200">{{ t('admin.promptAudit.conversations.responseContext') }}</summary>
      <p v-if="turn.request_details.response_context_truncated" class="mt-2 text-xs text-amber-700">{{ t('admin.promptAudit.conversations.contextTruncated') }}</p>
      <div class="mt-4 space-y-4"><ConversationMessage v-for="message in turn.request_details.response_context" :key="message.position" :message="message" /></div>
    </details>
    <p v-if="turn.categories.length" class="mt-3 text-sm text-red-700 dark:text-red-300">{{ turn.categories.join(', ') }}</p>
    <p v-if="turn.review_error_code" class="mt-3 text-sm text-amber-700 dark:text-amber-300">{{ turn.review_error_code }} · {{ turn.review_error_message }}</p>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import ConversationMessage from './ConversationMessage.vue'
import { isOpenClawHeartbeat } from '../conversationReading'
import type { ConversationTurn, ConversationMessage as Message } from '../types'
const props = defineProps<{ turn: ConversationTurn; timelineMessages?: Message[] }>()
const { t, locale } = useI18n()
const structured = computed(() => props.turn.request_details?.version === 1)
const auxiliary = computed(() => props.turn.request_kind === 'auxiliary')
const messages = computed(() => props.turn.request_details?.messages || [])
const context = computed(() => props.turn.request_details?.context || [])
const current = computed(() => props.timelineMessages ?? messages.value.filter(message => message.position >= (props.turn.request_details?.current_message_position ?? -1)))
const history = computed(() => messages.value.filter(message => message.position < (props.turn.request_details?.current_message_position ?? -1)))
const date = computed(() => new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(props.turn.captured_at)))
</script>
