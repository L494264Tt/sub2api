<template>
  <div class="space-y-6" data-test="reading-view">
    <OpenClawActivity :activities="openClawActivities" />
    <p v-if="!groups.some(group => group.question) && !openClawActivities.length" class="text-sm text-gray-500">{{ t('admin.promptAudit.conversations.noReadableDialogue') }}</p>
    <div v-else-if="groups.some(group => group.question)" class="border-b border-gray-200 pb-5 dark:border-dark-700">
      <h3 class="text-sm font-semibold text-gray-600 dark:text-dark-300">{{ t('admin.promptAudit.conversations.taskOriginal') }}</h3>
      <p class="mt-2 whitespace-pre-wrap break-words text-base leading-7 text-gray-900 dark:text-white [overflow-wrap:anywhere]">{{ groups.find(group => group.question)?.question?.content }}</p>
      <div class="mt-3 flex flex-wrap gap-4 text-xs text-gray-500">
        <span>{{ t('admin.promptAudit.conversations.userMessageCount', { count: groups.filter(group => group.question).length }) }}</span>
        <span>{{ t('admin.promptAudit.conversations.toolRequestCount', { count: toolRequests }) }}</span>
      </div>
      <h3 v-if="latestReply" class="mt-5 text-sm font-semibold text-gray-600 dark:text-dark-300">{{ t('admin.promptAudit.conversations.latestReply') }}</h3>
      <p v-if="latestReply" class="mt-2 whitespace-pre-wrap break-words text-sm leading-7 text-gray-900 dark:text-dark-100 [overflow-wrap:anywhere]">{{ latestReply.content }}</p>
      <p v-else class="mt-3 text-sm text-gray-500">{{ t('admin.promptAudit.conversations.noReplyYet') }}</p>
    </div>
    <details v-if="groups.length" data-test="dialogue-history">
      <summary class="cursor-pointer text-sm font-medium text-gray-600 dark:text-dark-200">{{ t('admin.promptAudit.conversations.dialogueHistory') }}</summary>
      <div class="mt-4 space-y-6">
    <section v-for="(group, index) in groups" :key="index" class="min-w-0 space-y-4 border-b border-gray-100 pb-5 dark:border-dark-800">
      <div v-if="group.question" class="border-l-2 border-primary-500 pl-3">
        <div class="text-xs font-semibold text-primary-600 dark:text-primary-400">{{ t('admin.promptAudit.conversations.roles.user') }}</div>
        <p class="mt-1 whitespace-pre-wrap break-words text-sm leading-7 text-gray-900 dark:text-dark-100 [overflow-wrap:anywhere]">{{ group.question.content }}</p>
        <span v-if="group.question.truncated" class="text-xs text-amber-700">{{ t('admin.promptAudit.conversations.messageTruncated') }}</span>
      </div>
      <details v-if="group.replies.length > 1" data-test="progress-records">
        <summary class="cursor-pointer text-xs text-gray-500">{{ t('admin.promptAudit.conversations.progressRecords', { count: group.replies.length - 1 }) }}</summary>
        <div class="mt-3 space-y-3"><ConversationMessage v-for="(reply, replyIndex) in group.replies.slice(0, -1)" :key="replyIndex" :message="reply" /></div>
      </details>
      <div v-if="group.replies.length" class="border-l-2 border-gray-200 pl-3 dark:border-dark-600">
        <div class="text-xs font-semibold text-gray-500">{{ t('admin.promptAudit.conversations.latestReply') }}</div>
        <p class="mt-1 whitespace-pre-wrap break-words text-sm leading-7 text-gray-900 dark:text-dark-100 [overflow-wrap:anywhere]">{{ group.replies.at(-1)?.content }}</p>
        <span v-if="group.replies.at(-1)?.truncated" class="text-xs text-amber-700">{{ t('admin.promptAudit.conversations.messageTruncated') }}</span>
      </div>
      <p v-else class="text-sm text-gray-500">{{ t('admin.promptAudit.conversations.noReplyYet') }}</p>
    </section>
      </div>
    </details>
    <p v-if="hasGaps" role="status" class="text-xs text-amber-700 dark:text-amber-300">{{ t('admin.promptAudit.conversations.readingGaps') }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { buildConversationTimeline } from '../conversationTimeline'
import { isAuxiliaryTurn, readingResponse } from '../conversationReading'
import ConversationMessage from './ConversationMessage.vue'
import OpenClawActivity from './OpenClawActivity.vue'
import { buildOpenClawActivity } from '../openClawActivity'
import type { ConversationMessage as Message, ConversationTurn } from '../types'
const props = defineProps<{ turns: ConversationTurn[] }>()
const { t } = useI18n()
const openClawActivities = computed(() => buildOpenClawActivity(props.turns))
const groups = computed(() => {
  const result: { question?: Message; replies: Message[] }[] = []
  for (const entry of buildConversationTimeline(props.turns)) {
    const items = [...entry.messages]
    const response = readingResponse(entry.turn)
    if (response) items.push({ role: 'assistant', kind: 'text', content: response, position: -1, truncated: entry.turn.response_truncated })
    for (const message of items) {
      if (message.role === 'user') result.push({ question: message, replies: [] })
      else if (message.role === 'assistant') {
        if (!result.length) result.push({ replies: [] })
        const group = result[result.length - 1]
        if (group.replies.at(-1)?.content.trim() !== message.content.trim()) group.replies.push(message)
      }
    }
  }
  return result
})
const toolRequests = computed(() => props.turns.filter(turn => !isAuxiliaryTurn(turn) && turn.request_details?.response_context?.some(message => /tool_use|function_call/.test(message.kind))).length)
const latestReply = computed(() => [...groups.value].reverse().find(group => group.replies.length)?.replies.at(-1))
const hasGaps = computed(() => props.turns.some(turn => turn.request_truncated || turn.response_truncated || turn.request_details?.version !== 1))
</script>
