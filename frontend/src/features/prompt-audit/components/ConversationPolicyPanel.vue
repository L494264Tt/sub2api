<template>
  <section aria-labelledby="conversation-policy-title" class="border-t border-gray-100 py-6 dark:border-dark-800">
    <div>
      <h2 id="conversation-policy-title" class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.promptAudit.conversations.configTitle') }}</h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('admin.promptAudit.conversations.configDescription') }}</p>
    </div>

    <div class="mt-5 grid gap-5 lg:grid-cols-[minmax(0,0.75fr)_minmax(0,1.25fr)]">
      <div class="space-y-4 rounded-lg border border-gray-200 p-4 dark:border-dark-700">
        <ToggleRow :label="t('admin.promptAudit.conversations.recording')" :model-value="draft.conversation_recording_enabled" @update:model-value="setRecording" />
        <ToggleRow :label="t('admin.promptAudit.conversations.periodicReview')" :model-value="draft.conversation_review_enabled" :disabled="!draft.enabled || !draft.conversation_recording_enabled" @update:model-value="patch({ conversation_review_enabled: $event })" />
        <p class="text-xs leading-5 text-gray-500 dark:text-dark-400">{{ t('admin.promptAudit.conversations.privacyHint') }}</p>
      </div>

      <div class="grid gap-4 rounded-lg border border-gray-200 p-4 dark:border-dark-700 sm:grid-cols-2 xl:grid-cols-3">
        <NumberField field="conversation_review_interval_minutes" :label="t('admin.promptAudit.conversations.interval')" :min="5" :max="10080" />
        <NumberField field="conversation_review_batch_size" :label="t('admin.promptAudit.conversations.batchSize')" :min="1" :max="500" />
        <NumberField field="conversation_retention_days" :label="t('admin.promptAudit.conversations.retention')" :min="1" :max="3650" />
        <NumberField field="conversation_request_max_runes" :label="t('admin.promptAudit.conversations.requestLimit')" :min="128" :max="500000" />
        <NumberField field="conversation_response_max_runes" :label="t('admin.promptAudit.conversations.responseLimit')" :min="128" :max="500000" />
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { defineComponent, h } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PromptAuditDraft } from '../types'
import { cloneData } from '../viewModel'

const props = defineProps<{ draft: PromptAuditDraft }>()
const emit = defineEmits<{ (event: 'update:draft', value: PromptAuditDraft): void }>()
const { t } = useI18n()

const ToggleRow = defineComponent({
  props: { label: { type: String, required: true }, modelValue: { type: Boolean, required: true }, disabled: { type: Boolean, default: false } },
  emits: ['update:modelValue'],
  setup(toggleProps, { emit: emitToggle }) {
    return () => h('button', {
      type: 'button', role: 'switch', 'aria-checked': toggleProps.modelValue, disabled: toggleProps.disabled,
      'aria-label': toggleProps.label,
      class: ['flex w-full items-center justify-between gap-3 text-left text-sm', toggleProps.disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'],
      onClick: () => emitToggle('update:modelValue', !toggleProps.modelValue),
    }, [
      h('span', { class: 'font-medium text-gray-800 dark:text-dark-100' }, toggleProps.label),
      h('span', { class: ['relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors', toggleProps.modelValue ? 'bg-primary-600' : 'bg-gray-300 dark:bg-dark-600'] }, [
        h('span', { class: ['h-5 w-5 rounded-full bg-white shadow transition-transform', toggleProps.modelValue ? 'translate-x-5' : 'translate-x-0.5'] }),
      ]),
    ])
  },
})

const NumberField = defineComponent({
  props: { field: { type: String, required: true }, label: { type: String, required: true }, min: { type: Number, required: true }, max: { type: Number, required: true } },
  setup(fieldProps) {
    return () => h('label', { class: 'block text-sm text-gray-700 dark:text-dark-200' }, [
      h('span', fieldProps.label),
      h('input', {
        type: 'number', min: fieldProps.min, max: fieldProps.max,
        value: props.draft[fieldProps.field as keyof PromptAuditDraft] as number,
        class: 'input mt-1.5 w-full',
        onInput: (event: Event) => patch({ [fieldProps.field]: Number((event.target as HTMLInputElement).value) } as Partial<PromptAuditDraft>),
      }),
    ])
  },
})

function patch(value: Partial<PromptAuditDraft>) {
  emit('update:draft', { ...cloneData(props.draft), ...value })
}

function setRecording(value: boolean) {
  patch({ conversation_recording_enabled: value, conversation_review_enabled: value ? props.draft.conversation_review_enabled : false })
}
</script>
