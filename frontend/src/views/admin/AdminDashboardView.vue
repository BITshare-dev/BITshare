<script setup lang="ts">
import { onMounted, ref } from "vue";

import AdminFolderTreeNode, { type FolderTreeNode } from "../../components/AdminFolderTreeNode.vue";
import { HttpError, httpClient } from "../../lib/http/client";
import { useSessionStore } from "../../stores/session";

interface AdminMeResponse {
  admin: {
    id: string;
    username: string;
    role: string;
    status: string;
    permissions: string[];
  };
}

interface PendingSubmissionItem {
  submission_id: string;
  receipt_code: string;
  title: string;
  description: string;
  status: "pending";
  uploaded_at: string;
  file_name: string;
  file_size: number;
  file_mime_type: string;
}

const sessionStore = useSessionStore();

const username = ref("superadmin");
const password = ref("");
const loginLoading = ref(false);
const loginError = ref("");

const pending = ref<PendingSubmissionItem[]>([]);
const pendingLoading = ref(false);
const pendingError = ref("");

const importRootPath = ref("");
const importLoading = ref(false);
const importMessage = ref("");
const importError = ref("");

const folderTree = ref<FolderTreeNode[]>([]);
const treeLoading = ref(false);
const treeError = ref("");
const tagInputs = ref<Record<string, string>>({});

onMounted(async () => {
  await restoreSession();
  if (sessionStore.authenticated) {
    await Promise.all([loadPending(), loadFolderTree()]);
  }
});

async function restoreSession() {
  try {
    const response = await httpClient.get<AdminMeResponse>("/admin/me");
    applySession(response);
  } catch {
    sessionStore.reset();
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
    password.value = "";
    await Promise.all([loadPending(), loadFolderTree()]);
  } catch (error: unknown) {
    if (error instanceof HttpError && typeof error.payload === "object" && error.payload && "error" in error.payload) {
      loginError.value = String(error.payload.error);
    } else {
      loginError.value = "登录失败，请重试。";
    }
  } finally {
    loginLoading.value = false;
  }
}

async function logout() {
  await httpClient.post("/admin/session/logout");
  sessionStore.reset();
  pending.value = [];
  folderTree.value = [];
}

async function loadPending() {
  pendingLoading.value = true;
  pendingError.value = "";

  try {
    const response = await httpClient.get<{ items: PendingSubmissionItem[] }>("/admin/submissions/pending");
    pending.value = response.items;
  } catch {
    pendingError.value = "加载待审核列表失败。";
  } finally {
    pendingLoading.value = false;
  }
}

async function approve(submissionId: string) {
  await httpClient.post(`/admin/submissions/${submissionId}/approve`);
  await loadPending();
}

async function reject(submissionId: string) {
  const rejectReason = window.prompt("请输入驳回原因");
  if (!rejectReason) {
    return;
  }
  await httpClient.post(`/admin/submissions/${submissionId}/reject`, {
    reject_reason: rejectReason,
  });
  await loadPending();
}

async function importLocalDirectory() {
  importLoading.value = true;
  importError.value = "";
  importMessage.value = "";

  try {
    const response = await httpClient.post<{
      root_path: string;
      imported_folders: number;
      imported_files: number;
      skipped_folders: number;
      skipped_files: number;
      conflicts: string[];
    }>("/admin/imports/local", {
      root_path: importRootPath.value,
    });

    importMessage.value = `导入完成：新增目录 ${response.imported_folders}，新增文件 ${response.imported_files}，跳过目录 ${response.skipped_folders}，跳过文件 ${response.skipped_files}`;
    if (response.conflicts.length > 0) {
      importMessage.value += `，冲突 ${response.conflicts.length} 项`;
    }
    await loadFolderTree();
  } catch (error: unknown) {
    if (error instanceof HttpError && typeof error.payload === "object" && error.payload && "error" in error.payload) {
      importError.value = String(error.payload.error);
    } else {
      importError.value = "导入失败，请检查路径和权限。";
    }
  } finally {
    importLoading.value = false;
  }
}

async function loadFolderTree() {
  treeLoading.value = true;
  treeError.value = "";

  try {
    const response = await httpClient.get<{ items: FolderTreeNode[] }>("/admin/folders/tree");
    folderTree.value = response.items;
    syncTagInputs(response.items);
  } catch {
    treeError.value = "加载目录树失败。";
  } finally {
    treeLoading.value = false;
  }
}

async function saveFolderTags(folderId: string) {
  const tags = (tagInputs.value[folderId] ?? "")
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);

  await httpClient.request(`/admin/folders/${folderId}/tags`, {
    method: "PUT",
    body: { tags },
  });
  await loadFolderTree();
}

function applySession(response: AdminMeResponse) {
  sessionStore.setAuthenticated(true, response.admin.username, {
    adminId: response.admin.id,
    role: response.admin.role,
    status: response.admin.status,
    permissions: response.admin.permissions,
  });
}

function syncTagInputs(nodes: FolderTreeNode[]) {
  const next: Record<string, string> = {};
  const visit = (list: FolderTreeNode[]) => {
    for (const node of list) {
      next[node.id] = node.tags.join(", ");
      visit(node.folders);
    }
  };
  visit(nodes);
  tagInputs.value = next;
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function formatSize(size: number) {
  if (size < 1024) {
    return `${size} B`;
  }
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`;
  }
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}
</script>

<template>
  <section class="space-y-8">
    <header class="flex flex-wrap items-center justify-between gap-4">
      <div>
        <p class="text-sm font-semibold uppercase tracking-[0.28em] text-blue-300">Dashboard</p>
        <h2 class="mt-3 text-3xl font-semibold text-white">最小测试控制台</h2>
      </div>

      <div
        v-if="sessionStore.authenticated"
        class="rounded-[24px] border border-slate-800 bg-slate-950/70 px-5 py-4 text-sm text-slate-300"
      >
        <p class="font-semibold text-white">{{ sessionStore.displayName }}</p>
        <p class="mt-1">{{ sessionStore.role }} / {{ sessionStore.status }}</p>
        <button
          class="mt-3 rounded-xl border border-slate-700 px-4 py-2 text-xs font-semibold text-slate-200 transition hover:bg-slate-800"
          @click="logout"
        >
          退出登录
        </button>
      </div>
    </header>

    <article
      v-if="!sessionStore.authenticated"
      class="max-w-xl rounded-[28px] border border-slate-800 bg-slate-950/70 p-6"
    >
      <p class="text-sm font-semibold uppercase tracking-[0.24em] text-blue-300">Admin Login</p>
      <h3 class="mt-2 text-2xl font-semibold text-white">先登录管理后台</h3>

      <form class="mt-6 space-y-4" @submit.prevent="login">
        <label class="block">
          <span class="mb-2 block text-sm font-medium text-slate-300">用户名</span>
          <input
            v-model="username"
            class="w-full rounded-2xl border border-slate-700 bg-slate-900 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
          />
        </label>
        <label class="block">
          <span class="mb-2 block text-sm font-medium text-slate-300">密码</span>
          <input
            v-model="password"
            type="password"
            class="w-full rounded-2xl border border-slate-700 bg-slate-900 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
          />
        </label>
        <button
          type="submit"
          class="rounded-2xl bg-blue-500 px-5 py-3 text-sm font-semibold text-slate-950 transition hover:bg-blue-400 disabled:cursor-not-allowed disabled:bg-slate-500"
          :disabled="loginLoading"
        >
          {{ loginLoading ? "登录中..." : "登录" }}
        </button>
      </form>

      <p v-if="loginError" class="mt-4 rounded-2xl bg-rose-950/50 px-4 py-3 text-sm text-rose-200">
        {{ loginError }}
      </p>
    </article>

    <template v-else>
      <div class="grid gap-6 xl:grid-cols-[1fr_1fr]">
        <article class="rounded-[28px] border border-slate-800 bg-slate-950/70 p-6">
          <div class="flex items-center justify-between gap-4">
            <div>
              <p class="text-sm font-semibold uppercase tracking-[0.22em] text-blue-300">Moderation</p>
              <h3 class="mt-2 text-2xl font-semibold text-white">待审核列表</h3>
            </div>
            <button
              class="rounded-2xl border border-slate-700 px-4 py-3 text-sm font-medium text-slate-200 transition hover:bg-slate-800"
              @click="loadPending"
            >
              刷新
            </button>
          </div>

          <p v-if="pendingError" class="mt-4 rounded-2xl bg-rose-950/50 px-4 py-3 text-sm text-rose-200">
            {{ pendingError }}
          </p>
          <p v-else-if="pendingLoading" class="mt-4 text-sm text-slate-400">加载中...</p>

          <div v-else class="mt-5 space-y-4">
            <article
              v-for="item in pending"
              :key="item.submission_id"
              class="rounded-[22px] border border-slate-800 bg-slate-900 p-4"
            >
              <div class="flex items-start justify-between gap-4">
                <div>
                  <h4 class="text-lg font-semibold text-white">{{ item.title }}</h4>
                  <p class="mt-2 text-sm text-slate-400">{{ item.file_name }} / {{ formatSize(item.file_size) }}</p>
                  <p class="mt-1 text-sm text-slate-500">回执码：{{ item.receipt_code }}</p>
                  <p class="mt-1 text-sm text-slate-500">上传于 {{ formatDate(item.uploaded_at) }}</p>
                </div>
                <div class="flex flex-col gap-2">
                  <button
                    class="rounded-xl bg-emerald-500 px-4 py-2 text-sm font-semibold text-slate-950 transition hover:bg-emerald-400"
                    @click="approve(item.submission_id)"
                  >
                    通过
                  </button>
                  <button
                    class="rounded-xl bg-rose-500 px-4 py-2 text-sm font-semibold text-white transition hover:bg-rose-400"
                    @click="reject(item.submission_id)"
                  >
                    驳回
                  </button>
                </div>
              </div>

              <p v-if="item.description" class="mt-3 rounded-2xl bg-slate-950 px-4 py-3 text-sm text-slate-300">
                {{ item.description }}
              </p>
            </article>

            <p
              v-if="pending.length === 0"
              class="rounded-2xl border border-dashed border-slate-700 px-4 py-6 text-center text-sm text-slate-400"
            >
              当前没有待审核投稿。
            </p>
          </div>
        </article>

        <article class="rounded-[28px] border border-slate-800 bg-slate-950/70 p-6">
          <div>
            <p class="text-sm font-semibold uppercase tracking-[0.22em] text-blue-300">Import</p>
            <h3 class="mt-2 text-2xl font-semibold text-white">本地目录导入</h3>
          </div>

          <form class="mt-6 space-y-4" @submit.prevent="importLocalDirectory">
            <label class="block">
              <span class="mb-2 block text-sm font-medium text-slate-300">服务器绝对路径</span>
              <input
                v-model="importRootPath"
                placeholder="/Users/quan/Desktop/OpenShare/fixtures/import-root"
                class="w-full rounded-2xl border border-slate-700 bg-slate-900 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
              />
            </label>

            <button
              type="submit"
              class="rounded-2xl bg-blue-500 px-5 py-3 text-sm font-semibold text-slate-950 transition hover:bg-blue-400 disabled:cursor-not-allowed disabled:bg-slate-500"
              :disabled="importLoading"
            >
              {{ importLoading ? "导入中..." : "开始导入" }}
            </button>
          </form>

          <p v-if="importMessage" class="mt-4 rounded-2xl bg-emerald-950/40 px-4 py-3 text-sm text-emerald-200">
            {{ importMessage }}
          </p>
          <p v-if="importError" class="mt-4 rounded-2xl bg-rose-950/50 px-4 py-3 text-sm text-rose-200">
            {{ importError }}
          </p>
        </article>
      </div>

      <article class="rounded-[28px] border border-slate-800 bg-slate-950/70 p-6">
        <div class="flex items-center justify-between gap-4">
          <div>
            <p class="text-sm font-semibold uppercase tracking-[0.22em] text-blue-300">Folders</p>
            <h3 class="mt-2 text-2xl font-semibold text-white">目录树与 Tag</h3>
          </div>
          <button
            class="rounded-2xl border border-slate-700 px-4 py-3 text-sm font-medium text-slate-200 transition hover:bg-slate-800"
            @click="loadFolderTree"
          >
            刷新
          </button>
        </div>

        <p v-if="treeError" class="mt-4 rounded-2xl bg-rose-950/50 px-4 py-3 text-sm text-rose-200">{{ treeError }}</p>
        <p v-else-if="treeLoading" class="mt-4 text-sm text-slate-400">加载中...</p>

        <div v-else class="mt-5 space-y-4">
          <AdminFolderTreeNode
            v-for="node in folderTree"
            :key="node.id"
            :node="node"
            :tag-inputs="tagInputs"
            @save-tags="saveFolderTags"
          />

          <p
            v-if="folderTree.length === 0"
            class="rounded-2xl border border-dashed border-slate-700 px-4 py-6 text-center text-sm text-slate-400"
          >
            还没有导入任何目录。
          </p>
        </div>
      </article>
    </template>
  </section>
</template>
