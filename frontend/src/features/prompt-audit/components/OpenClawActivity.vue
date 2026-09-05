<template>
  <section v-if="latest" class="space-y-4 border-b border-gray-200 pb-5 dark:border-dark-700" data-test="openclaw-activity">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.promptAudit.openClaw.title') }}</h3>
      <span class="text-xs text-gray-500">{{ formatDate(latest.turn.captured_at) }}</span>
    </div>
    <dl class="grid gap-4 text-sm sm:grid-cols-2">
      <div><dt class="text-gray-500">{{ t('admin.promptAudit.openClaw.trigger') }}</dt><dd class="mt-1 text-gray-900 dark:text-dark-100">{{ t(latest.heartbeat ? 'admin.promptAudit.openClaw.heartbeat' : 'admin.promptAudit.openClaw.modelRequest') }}</dd></div>
      <div v-if="latest.heartbeatAcknowledged"><dt class="text-gray-500">{{ t('admin.promptAudit.openClaw.receipt') }}</dt><dd class="mt-1 text-gray-900 dark:text-dark-100">{{ t('admin.promptAudit.openClaw.acknowledged') }} <code class="text-xs">HEARTBEAT_OK</code></dd></div>
      <div class="sm:col-span-2"><dt class="text-gray-500">{{ t('admin.promptAudit.openClaw.requestedTools') }}</dt><dd class="mt-1 break-words text-gray-900 dark:text-dark-100 [overflow-wrap:anywhere]">{{ latest.requestedTools.length ? latest.requestedTools.join(', ') : t(latest.fragmentedCalls ? 'admin.promptAudit.openClaw.partialCalls' : 'admin.promptAudit.openClaw.noCalls') }}</dd></div>
    </dl>
    <p v-if="latest.heartbeatAcknowledged" class="text-sm text-gray-600 dark:text-dark-300">{{ t('admin.promptAudit.openClaw.heartbeatMeaning') }}</p>
    <p v-if="latest.requestedTools.length" class="text-xs text-gray-500">{{ t('admin.promptAudit.openClaw.requestNotCompletion') }}</p>
    <p v-if="latest.turn.response_truncated || latest.turn.request_details?.response_context_truncated" class="text-xs text-amber-700 dark:text-amber-300">{{ t('admin.promptAudit.openClaw.incomplete') }}</p>
    <details v-if="activities.length > 1">
      <summary class="cursor-pointer text-sm text-gray-600 dark:text-dark-200">{{ t('admin.promptAudit.openClaw.history', { count: activities.length }) }}</summary>
      <ol class="mt-3 space-y-3 text-sm">
        <li v-for="activity in activities" :key="activity.turn.id" class="border-l-2 border-gray-200 pl-3 dark:border-dark-600">
          <div class="text-xs text-gray-500">{{ formatDate(activity.turn.captured_at) }}</div>
          <div class="mt-1 text-gray-900 dark:text-dark-100">{{ t(activity.heartbeat ? 'admin.promptAudit.openClaw.heartbeat' : 'admin.promptAudit.openClaw.modelRequest') }}</div>
          <div class="mt-1 break-words text-xs text-gray-500">{{ activity.requestedTools.length ? activity.requestedTools.join(', ') : t(activity.fragmentedCalls ? 'admin.promptAudit.openClaw.partialCalls' : 'admin.promptAudit.openClaw.noCalls') }}</div>
        </li>
      </ol>
    </details>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpenClawActivity } from '../openClawActivity'
const props = defineProps<{ activities: OpenClawActivity[] }>()
const { t, locale } = useI18n()
const latest = computed(() => props.activities[0])
function formatDate(value: string) { return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) }
</script>
