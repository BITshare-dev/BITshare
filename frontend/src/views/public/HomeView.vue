<template>
  <section class="grid gap-6 lg:grid-cols-[1.2fr_0.8fr]">
    <article class="rounded-[28px] bg-slate-950 px-8 py-10 text-white">
      <p class="text-sm font-semibold uppercase tracking-[0.28em] text-blue-300">Public Portal</p>
      <h2 class="mt-4 max-w-xl text-4xl font-semibold leading-tight">
        资料浏览、搜索、下载的用户端骨架已经就绪。
      </h2>
      <p class="mt-4 max-w-2xl text-base text-slate-300">
        当前阶段先完成基础布局、路由拆分和会话透传能力，后续功能页将继续按开发规划逐步填充。
      </p>
    </article>

    <article class="rounded-[28px] border border-slate-200 bg-white px-6 py-7">
      <p class="text-sm font-semibold uppercase tracking-[0.24em] text-blue-700">Infrastructure</p>
      <ul class="mt-5 space-y-3 text-sm text-slate-600">
        <li>Vue Router 已拆分用户端与管理端布局。</li>
        <li>Pinia 已接入，可承接会话与页面状态。</li>
        <li>HTTP 请求模块默认携带 Cookie。</li>
      </ul>
    </article>
  </section>
</template>
