<template>
  <section class="space-y-6">
    <header>
      <p class="text-sm font-semibold uppercase tracking-[0.28em] text-blue-300">Dashboard</p>
      <h2 class="mt-3 text-3xl font-semibold text-white">管理端布局与基础状态已初始化</h2>
    </header>

    <div class="grid gap-4 md:grid-cols-3">
      <article
        v-for="item in cards"
        :key="item.title"
        class="rounded-[24px] border border-slate-800 bg-slate-950/70 p-5"
      >
        <p class="text-sm text-slate-400">{{ item.label }}</p>
        <h3 class="mt-2 text-lg font-semibold text-white">{{ item.title }}</h3>
        <p class="mt-3 text-sm text-slate-300">{{ item.description }}</p>
      </article>
    </div>
  </section>
</template>

<script setup lang="ts">
const cards = [
  {
    label: "Router",
    title: "双布局路由",
    description: "用户端与管理端已经拆分到独立布局，后续页面可以按模块增量扩展。",
  },
  {
    label: "Session",
    title: "Cookie 透传",
    description: "请求封装默认开启 credentials，后续后台登录态将沿用这一约定。",
  },
  {
    label: "State",
    title: "Pinia 基础仓库",
    description: "会话状态与全局配置状态已有统一承载点，避免散落在页面组件中。",
  },
] as const;
</script>
