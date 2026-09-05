<template>
  <div class="min-w-0 border-l-2 border-gray-200 pl-3 dark:border-dark-600">
    <div class="mb-1 flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-dark-300">
      <span class="font-semibold">{{ roleLabel }}</span>
      <span v-if="message.kind !== 'text'">{{ message.kind }}</span>
      <span v-if="message.truncated" class="text-amber-700 dark:text-amber-300">{{ t('admin.promptAudit.conversations.messageTruncated') }}</span>
    </div>
    <pre class="max-h-96 overflow-auto whitespace-pre-wrap break-words text-sm leading-6 text-gray-900 dark:text-dark-100 [overflow-wrap:anywhere]">{{ message.content }}</pre>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ConversationMessage } from '../types'
const props = defineProps<{ message: ConversationMessage }>()
const { t } = useI18n()
const roleLabel = computed(() => ['user', 'assistant', 'system', 'developer', 'tool'].includes(props.message.role)
  ? t(`admin.promptAudit.conversations.roles.${props.message.role}`) : props.message.role)
</script>
