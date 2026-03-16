<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RouterLink, RouterView, useRoute } from "vue-router";
import { LayoutDashboard, Inbox, ScrollText, Shield, UserRound } from "lucide-vue-next";

import AdminSidebar, { type AdminSidebarItem } from "../components/AdminSidebar.vue";
import { HttpError, httpClient } from "../lib/http/client";
import { useSessionStore } from "../stores/session";

interface AdminMeResponse {
  admin: {
    id: string;
    username: string;
    display_name: string;
    avatar_url: string;
    role: string;
    status: string;
    permissions: string[];
  };
}

const sessionStore = useSessionStore();
const route = useRoute();

const username = ref("");
const password = ref("");
const loading = ref(true);
const loginLoading = ref(false);
const loginError = ref("");

const navItems = computed<AdminSidebarItem[]>(() => [
  { to: "/admin/audit", label: "审核", icon: Inbox },
  { to: "/admin/logs", label: "操作记录", icon: ScrollText },
  { to: "/admin/permissions", label: "权限管理", icon: Shield },
]);

onMounted(async () => {
  await restoreSession();
});

async function restoreSession() {
  loading.value = true;
  try {
    const response = await httpClient.get<AdminMeResponse>("/admin/me");
    applySession(response);
    await trackVisit();
  } catch {
    sessionStore.reset();
  } finally {
    loading.value = false;
  }
}

async function login() {
  loginLoading.value = true;
  loginError.value = "";

  try {
    const response = await httpClient.post<AdminMeResponse>("/admin/session/login", {
      username: username.value,
      password: password.value,
    });
    applySession(response);
    await trackVisit();
    password.value = "";
  } catch (error: unknown) {
    loginError.value = readApiError(error) ?? "登录失败，请重试。";
  } finally {
    loginLoading.value = false;
  }
}

async function logout() {
  await httpClient.post("/admin/session/logout");
  sessionStore.reset();
}

function applySession(response: AdminMeResponse) {
  sessionStore.setAuthenticated(true, response.admin.display_name || response.admin.username, {
    username: response.admin.username,
    adminId: response.admin.id,
    avatarUrl: response.admin.avatar_url,
    role: response.admin.role,
    status: response.admin.status,
    permissions: response.admin.permissions,
  });
}

function readApiError(error: unknown) {
  if (!(error instanceof HttpError) || typeof error.payload !== "object" || error.payload === null) {
    return null;
  }
  const payload = error.payload as Record<string, unknown>;
  return typeof payload.error === "string" ? payload.error : null;
}

async function trackVisit() {
  try {
    await httpClient.request("/visits", {
      method: "POST",
      body: {
        scope: "admin",
        path: route.path,
      },
    });
  } catch {
    // Ignore analytics failures.
  }
}
</script>

<template>
  <div class="app-shell">
    <div v-if="loading" class="flex min-h-screen items-center justify-center">
      <p class="text-sm text-slate-500">正在加载管理后台…</p>
    </div>

    <div v-else-if="!sessionStore.authenticated" class="app-container flex min-h-screen items-center justify-center py-16">
      <section class="panel w-full max-w-[420px] p-8">
        <div class="space-y-2">
          <p class="text-sm font-semibold text-slate-600 dark:text-slate-400">OpenShare Admin</p>
          <h2 class="text-3xl font-semibold tracking-tight text-slate-900 dark:text-slate-100">管理员登录</h2>
        </div>

        <form class="mt-8 space-y-4" @submit.prevent="login">
            <div class="space-y-2">
              <label class="text-sm font-medium text-slate-700 dark:text-slate-300">标示ID</label>
              <input v-model="username" class="field" placeholder="请输入标示ID" />
            </div>
            <div class="space-y-2">
              <label class="text-sm font-medium text-slate-700 dark:text-slate-300">密码</label>
              <input v-model="password" type="password" class="field" placeholder="请输入密码" />
            </div>

            <button type="submit" class="btn-primary h-11 w-full" :disabled="loginLoading">
              {{ loginLoading ? "登录中…" : "登录后台" }}
            </button>

            <RouterLink
              to="/"
              class="flex h-11 w-full items-center justify-center rounded-xl border border-slate-200 bg-white text-sm font-semibold text-slate-900 transition hover:border-slate-300 hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100 dark:hover:border-slate-600 dark:hover:bg-slate-800"
            >
              返回前台
            </RouterLink>
          </form>

        <p v-if="loginError" class="mt-4 rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
            {{ loginError }}
        </p>

      </section>
    </div>

    <div v-else class="flex min-h-screen bg-[#fafafa] dark:bg-slate-950">
      <aside class="fixed inset-y-0 left-0 z-20 w-[240px]">
        <AdminSidebar
          :current-path="route.path"
          :items="[{ label: '控制台', to: '/admin', icon: LayoutDashboard }, ...navItems]"
          :title="sessionStore.displayName"
          subtitle=""
          :avatar-url="sessionStore.avatarUrl"
          :avatar-fallback="sessionStore.displayName.slice(0, 1).toUpperCase() || 'A'"
          @logout="logout"
        >
          <template #footer-actions>
            <RouterLink
              to="/admin/account"
              class="flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition"
              :class="route.path.startsWith('/admin/account') ? 'bg-slate-200/70 text-slate-900 dark:bg-slate-800 dark:text-slate-100' : 'text-slate-600 hover:bg-slate-100 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-slate-900 dark:hover:text-slate-100'"
            >
              <UserRound class="h-4 w-4 shrink-0" />
              <span>账号设置</span>
            </RouterLink>
          </template>
        </AdminSidebar>
      </aside>

      <div class="min-w-0 flex-1 pl-[240px]">
        <main class="mx-auto w-full max-w-[1240px] px-6 py-8 lg:px-8">
          <RouterView />
        </main>
      </div>
    </div>
  </div>
</template>
