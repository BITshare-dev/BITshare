<script setup lang="ts">
withDefaults(
  defineProps<{
    title: string;
    value: string | number;
    hint?: string;
  }>(),
  {
    hint: "",
  },
);
</script>

<template>
  <article class="panel px-5 py-4">
    <div class="flex items-start justify-between gap-4">
      <div class="min-w-0">
        <p class="text-sm font-medium text-slate-500 dark:text-slate-400">{{ title }}</p>
        <p class="mt-2 text-[30px] font-semibold tracking-tight text-slate-900 dark:text-slate-100">
          {{ value }}
        </p>
        <p v-if="hint" class="mt-2 text-sm text-slate-500 dark:text-slate-400">
          {{ hint }}
        </p>
      </div>

      <div v-if="$slots.icon" class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-slate-200 bg-[#fafafa] text-slate-500 dark:border-slate-800 dark:bg-slate-800 dark:text-slate-400">
        <slot name="icon" />
      </div>
    </div>
  </article>
</template>
