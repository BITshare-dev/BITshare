<template>
  <div class="min-h-screen px-4 py-6 sm:px-6 lg:px-8">
    <div class="mx-auto flex min-h-[calc(100vh-3rem)] max-w-6xl flex-col rounded-[28px] border border-slate-200/70 bg-white/85 shadow-panel backdrop-blur">
      <header class="flex items-center justify-between border-b border-slate-200/80 px-6 py-5">
        <div>
          <p class="text-xs font-semibold uppercase tracking-[0.3em] text-blue-700">OpenShare</p>
          <h1 class="mt-1 text-xl font-semibold text-slate-900">资料共享平台</h1>
        </div>
        <nav class="flex items-center gap-3 text-sm text-slate-600">
          <RouterLink class="rounded-full px-4 py-2 transition hover:bg-slate-100 hover:text-slate-900" to="/">首页</RouterLink>
          <RouterLink class="rounded-full px-4 py-2 transition hover:bg-slate-100 hover:text-slate-900" to="/search">搜索</RouterLink>
          <RouterLink class="rounded-full px-4 py-2 transition hover:bg-slate-100 hover:text-slate-900" to="/admin">管理端</RouterLink>
        </nav>
      </header>

      <main class="flex-1 px-6 py-8">
        <RouterView />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { RouterLink, RouterView } from "vue-router";
</script>
