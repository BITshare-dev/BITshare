<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";

import SurfaceCard from "./ui/SurfaceCard.vue";
import { httpClient } from "../lib/http/client";
import { readApiError } from "../lib/http/helpers";

interface SystemPolicy {
  guest: {
    allow_direct_publish: boolean;
    extra_permissions_enabled: boolean;
    allow_guest_edit_title: boolean;
    allow_guest_edit_tags: boolean;
    allow_guest_edit_description: boolean;
    allow_guest_resource_delete: boolean;
  };
  upload: {
    max_file_size_bytes: number;
    max_tag_count: number;
    allowed_extensions: string[];
  };
  search: {
    enable_fuzzy_match: boolean;
    enable_tag_filter: boolean;
    enable_folder_scope: boolean;
    result_window: number;
  };
}

const loading = ref(false);
const loaded = ref(false);
const guestSaving = ref(false);
const uploadSaving = ref(false);
const error = ref("");
const message = ref("");
const importPath = ref("");
const importCurrentPath = ref("");
const importParentPath = ref("");
const importItems = ref<Array<{ name: string; path: string }>>([]);
const importLoading = ref(false);
const importMessage = ref("");
const importError = ref("");
const directoryPickerOpen = ref(false);
const pendingImportPath = ref("");
const confirmedImportPath = ref("");
const uploadSizeValue = ref(5);
const uploadSizeUnit = ref<"B" | "KB" | "MB" | "GB">("GB");
const guestSnapshot = ref("");
const uploadSnapshot = ref("");
const form = reactive<SystemPolicy>({
  guest: {
    allow_direct_publish: false,
    extra_permissions_enabled: false,
    allow_guest_edit_title: false,
    allow_guest_edit_tags: false,
    allow_guest_edit_description: false,
    allow_guest_resource_delete: false,
  },
  upload: {
    max_file_size_bytes: 0,
    max_tag_count: 0,
    allowed_extensions: [],
  },
  search: {
    enable_fuzzy_match: true,
    enable_tag_filter: true,
    enable_folder_scope: true,
    result_window: 100,
  },
});

onMounted(() => {
  void Promise.all([loadPolicy(), loadDirectories("")]);
});

async function loadPolicy() {
  loading.value = true;
  error.value = "";
  message.value = "";
  try {
    const response = await httpClient.get<SystemPolicy>("/admin/system/settings");
    Object.assign(form.guest, response.guest);
    Object.assign(form.upload, response.upload);
    Object.assign(form.search, response.search);
    applyUploadSizeFields(response.upload.max_file_size_bytes);
    guestSnapshot.value = serializeGuestState();
    uploadSnapshot.value = serializeUploadState();
  } catch {
    error.value = "加载系统设置失败。";
  } finally {
    loaded.value = true;
    loading.value = false;
  }
}

async function saveGuestPolicy() {
  guestSaving.value = true;
  error.value = "";
  message.value = "";
  form.guest.extra_permissions_enabled =
    form.guest.allow_guest_edit_title ||
    form.guest.allow_guest_edit_tags ||
    form.guest.allow_guest_edit_description ||
    form.guest.allow_guest_resource_delete;
  applyBuiltinSearchPolicy();

  try {
    await httpClient.request("/admin/system/settings", {
      method: "PUT",
      body: form,
    });
    guestSnapshot.value = serializeGuestState();
    message.value = "访客策略已更新。";
  } catch (err: unknown) {
    error.value = readApiError(err, "更新访客策略失败。");
  } finally {
    guestSaving.value = false;
  }
}

async function saveUploadPolicy() {
  uploadSaving.value = true;
  error.value = "";
  message.value = "";
  form.guest.extra_permissions_enabled =
    form.guest.allow_guest_edit_title ||
    form.guest.allow_guest_edit_tags ||
    form.guest.allow_guest_edit_description ||
    form.guest.allow_guest_resource_delete;
  form.upload.max_file_size_bytes = toBytes(uploadSizeValue.value, uploadSizeUnit.value);
  form.upload.max_tag_count = 0;
  form.upload.allowed_extensions = [];
  applyBuiltinSearchPolicy();

  try {
    await httpClient.request("/admin/system/settings", {
      method: "PUT",
      body: form,
    });
    uploadSnapshot.value = serializeUploadState();
    message.value = "上传限制已更新。";
  } catch (err: unknown) {
    error.value = readApiError(err, "更新上传限制失败。");
  } finally {
    uploadSaving.value = false;
  }
}

function applyBuiltinSearchPolicy() {
  form.search.enable_fuzzy_match = true;
  form.search.enable_tag_filter = true;
  form.search.enable_folder_scope = true;
  form.search.result_window = 100;
}

function serializeGuestState() {
  return JSON.stringify({
    allow_direct_publish: form.guest.allow_direct_publish,
    allow_guest_edit_title: form.guest.allow_guest_edit_title,
    allow_guest_edit_tags: form.guest.allow_guest_edit_tags,
    allow_guest_edit_description: form.guest.allow_guest_edit_description,
    allow_guest_resource_delete: form.guest.allow_guest_resource_delete,
  });
}

function serializeUploadState() {
  return JSON.stringify({
    max_file_size_bytes: toBytes(uploadSizeValue.value, uploadSizeUnit.value),
  });
}

function applyUploadSizeFields(bytes: number) {
  if (bytes >= 1024 * 1024 * 1024 && bytes % (1024 * 1024 * 1024) === 0) {
    uploadSizeValue.value = bytes / (1024 * 1024 * 1024);
    uploadSizeUnit.value = "GB";
    return;
  }
  if (bytes >= 1024 * 1024 && bytes % (1024 * 1024) === 0) {
    uploadSizeValue.value = bytes / (1024 * 1024);
    uploadSizeUnit.value = "MB";
    return;
  }
  if (bytes >= 1024 && bytes % 1024 === 0) {
    uploadSizeValue.value = bytes / 1024;
    uploadSizeUnit.value = "KB";
    return;
  }
  uploadSizeValue.value = bytes;
  uploadSizeUnit.value = "B";
}

function toBytes(value: number, unit: "B" | "KB" | "MB" | "GB") {
  const normalized = Math.max(1, Math.floor(value || 0));
  switch (unit) {
    case "GB":
      return normalized * 1024 * 1024 * 1024;
    case "MB":
      return normalized * 1024 * 1024;
    case "KB":
      return normalized * 1024;
    default:
      return normalized;
  }
}

const guestDirty = computed(() => loaded.value && guestSnapshot.value !== serializeGuestState());
const uploadDirty = computed(() => loaded.value && uploadSnapshot.value !== serializeUploadState());

async function loadDirectories(path: string) {
  importLoading.value = true;
  importError.value = "";
  try {
    const suffix = path ? `?path=${encodeURIComponent(path)}` : "";
    const response = await httpClient.get<{
      current_path: string;
      parent_path: string;
      items: Array<{ name: string; path: string }>;
    }>(`/admin/imports/directories${suffix}`);
    importCurrentPath.value = response.current_path;
    importParentPath.value = response.parent_path;
    importItems.value = response.items ?? [];
    if (!importPath.value) {
      importPath.value = response.current_path;
    }
  } catch (err: unknown) {
    importError.value = readApiError(err, "加载目录浏览器失败。");
  } finally {
    importLoading.value = false;
  }
}

async function openDirectoryPicker() {
  directoryPickerOpen.value = true;
  pendingImportPath.value = importPath.value.trim();
  await loadDirectories(importPath.value.trim());
  if (!pendingImportPath.value) {
    pendingImportPath.value = importCurrentPath.value;
  }
}

function closeDirectoryPicker() {
  directoryPickerOpen.value = false;
}

function selectCurrentDirectory() {
  confirmedImportPath.value = pendingImportPath.value || importCurrentPath.value;
  importPath.value = confirmedImportPath.value;
  directoryPickerOpen.value = false;
}

async function browseDirectory(path: string) {
  pendingImportPath.value = path;
  await loadDirectories(path);
}

async function importDirectory() {
  if (!importPath.value.trim()) {
    importError.value = "请先选择服务器目录。";
    return;
  }
  importLoading.value = true;
  importError.value = "";
  importMessage.value = "";
  try {
    const response = await httpClient.post<{
      imported_folders: number;
      imported_files: number;
    }>("/admin/imports/local", {
      root_path: importPath.value.trim(),
    });
    importMessage.value = `导入完成：${response.imported_folders} 个目录，${response.imported_files} 个文件。`;
    confirmedImportPath.value = "";
    importPath.value = "";
  } catch (err: unknown) {
    importError.value = readApiError(err, "导入目录失败。");
  } finally {
    importLoading.value = false;
  }
}
</script>

<template>
  <section class="space-y-4">
    <div>
      <h2 class="text-lg font-semibold tracking-tight text-slate-900">系统配置</h2>
    </div>

    <div v-if="!loaded && loading" class="text-sm text-slate-500">加载中…</div>

    <div v-else class="grid gap-6 xl:grid-cols-3">
      <form class="panel space-y-6 p-6" @submit.prevent="saveGuestPolicy">
        <div>
          <h3 class="text-lg font-semibold text-slate-900">访客策略</h3>
        </div>
        <div class="grid gap-3">
          <label class="panel-muted flex items-center gap-3 p-4 text-sm text-slate-700"><input v-model="form.guest.allow_direct_publish" type="checkbox" />允许游客免审核上传</label>
          <label class="panel-muted flex items-center gap-3 p-4 text-sm text-slate-700"><input v-model="form.guest.allow_guest_edit_title" type="checkbox" />允许访客编辑文件名</label>
          <label class="panel-muted flex items-center gap-3 p-4 text-sm text-slate-700"><input v-model="form.guest.allow_guest_edit_tags" type="checkbox" />允许访客编辑 Tag</label>
          <label class="panel-muted flex items-center gap-3 p-4 text-sm text-slate-700"><input v-model="form.guest.allow_guest_edit_description" type="checkbox" />允许访客编辑文件描述</label>
          <label class="panel-muted flex items-center gap-3 p-4 text-sm text-slate-700"><input v-model="form.guest.allow_guest_resource_delete" type="checkbox" />允许访客删除资料</label>
        </div>
        <button type="submit" class="btn-primary" :disabled="guestSaving || !guestDirty">
          {{ guestSaving ? "更新中…" : "确认更新" }}
        </button>
      </form>

      <form class="panel space-y-6 p-6" @submit.prevent="saveUploadPolicy">
        <div>
          <h3 class="text-lg font-semibold text-slate-900">上传限制</h3>
        </div>
        <div class="grid gap-4 md:grid-cols-[minmax(0,1fr)_140px]">
          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">最大上传大小</label>
            <input v-model.number="uploadSizeValue" type="number" min="1" class="field" placeholder="请输入大小" />
          </div>
          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">单位</label>
            <select v-model="uploadSizeUnit" class="field">
              <option value="GB">GB</option>
              <option value="MB">MB</option>
              <option value="KB">KB</option>
              <option value="B">B</option>
            </select>
          </div>
        </div>
        <button type="submit" class="btn-primary" :disabled="uploadSaving || !uploadDirty">
          {{ uploadSaving ? "更新中…" : "确认更新" }}
        </button>
      </form>

      <SurfaceCard class="space-y-6">
        <div>
          <h3 class="text-lg font-semibold text-slate-900">本地目录导入</h3>
        </div>
        <div class="space-y-4">
          <div class="rounded-xl border border-slate-200 bg-slate-50/70 px-4 py-3">
            <p class="text-xs font-medium uppercase tracking-[0.12em] text-slate-400">已选目录</p>
            <p class="mt-2 break-all text-sm text-slate-700">{{ importPath || "尚未选择服务器目录" }}</p>
          </div>
        </div>
        <div class="space-y-3">
          <button type="button" class="btn-secondary w-full" :disabled="importLoading" @click="openDirectoryPicker">
            选择服务器目录
          </button>
          <button type="button" class="btn-primary w-full" :disabled="importLoading || !confirmedImportPath.trim()" @click="importDirectory">
            {{ importLoading ? "导入中…" : "确认导入" }}
          </button>
        </div>
      </SurfaceCard>
    </div>

    <p v-if="message" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ message }}</p>
    <p v-if="error" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ error }}</p>
    <p v-if="importMessage" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ importMessage }}</p>
    <p v-if="importError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ importError }}</p>

    <div v-if="directoryPickerOpen" class="fixed inset-0 z-40 flex items-center justify-center bg-slate-950/30 px-4 py-8 backdrop-blur-sm">
      <SurfaceCard class="max-h-[80vh] w-full max-w-3xl overflow-hidden">
        <div class="flex items-start justify-between gap-4 border-b border-slate-200 pb-4">
          <div>
            <h3 class="text-lg font-semibold text-slate-900">选择服务器目录</h3>
            <p class="mt-1 text-sm text-slate-500">浏览服务器目录，确认后将当前目录作为导入源。</p>
          </div>
          <button type="button" class="btn-secondary" @click="closeDirectoryPicker">关闭</button>
        </div>

        <div class="mt-4 space-y-4">
          <div class="rounded-xl border border-slate-200 bg-slate-50/70 px-4 py-3">
            <p class="text-xs font-medium uppercase tracking-[0.12em] text-slate-400">当前目录</p>
            <p class="mt-2 break-all text-sm text-slate-700">{{ importCurrentPath || "加载中…" }}</p>
          </div>

          <div class="flex items-center justify-between gap-3">
            <button v-if="importParentPath" type="button" class="btn-secondary" @click="browseDirectory(importParentPath)">上一级</button>
            <div v-else></div>
            <button type="button" class="btn-primary" :disabled="importLoading || !(pendingImportPath || importCurrentPath)" @click="selectCurrentDirectory">
              选择当前目录
            </button>
          </div>

          <div class="max-h-[42vh] overflow-y-auto rounded-xl border border-slate-200 p-3">
            <div v-if="importLoading" class="py-6 text-center text-sm text-slate-500">目录加载中…</div>
            <div v-else-if="importItems.length === 0" class="py-6 text-center text-sm text-slate-500">当前目录下没有可浏览的子目录。</div>
            <div v-else class="space-y-2">
              <button
                v-for="item in importItems"
                :key="item.path"
                type="button"
                class="flex w-full items-center justify-between rounded-lg border border-slate-200 px-3 py-2.5 text-left text-sm text-slate-600 transition hover:bg-slate-50 hover:text-slate-900"
                @click="browseDirectory(item.path)"
              >
                <span>{{ item.name }}</span>
                <span class="text-xs text-slate-400">打开</span>
              </button>
            </div>
          </div>
        </div>
      </SurfaceCard>
    </div>
  </section>
</template>
