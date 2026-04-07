<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";

import SurfaceCard from "../ui/SurfaceCard.vue";
import { httpClient } from "../../lib/http/client";
import { readApiError } from "../../lib/http/helpers";

interface SystemPolicy {
  upload: {
    max_upload_total_bytes: number;
  };
}

interface ManagedFolderNode {
  id: string;
  name: string;
  source_path: string;
  folders: ManagedFolderNode[];
}

const loading = ref(false);
const loaded = ref(false);
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
const manualBrowsePath = ref("");
const confirmedImportPath = ref("");
const importFilter = ref("");
const managedFolders = ref<Array<{ id: string; name: string; sourcePath: string }>>([]);
const managedFoldersLoading = ref(false);
const managedFoldersError = ref("");
const unmanagingFolderID = ref("");
const unmanagePassword = ref("");
const unmanageError = ref("");
const unmanageMessage = ref("");
const rescanningFolderID = ref("");
const rescanError = ref("");
const rescanMessage = ref("");
const uploadSizeValue = ref(5);
const uploadSizeUnit = ref<"B" | "KB" | "MB" | "GB">("GB");
const uploadSnapshot = ref("");
const form = reactive<SystemPolicy>({
  upload: {
    max_upload_total_bytes: 0,
  },
});

onMounted(() => {
  void Promise.all([loadPolicy(), loadDirectories(""), loadManagedFolders()]);
});

async function loadPolicy() {
  loading.value = true;
  error.value = "";
  message.value = "";
  try {
    const response = await httpClient.get<SystemPolicy>("/admin/system/settings");
    Object.assign(form.upload, response.upload);
    applyUploadSizeFields(response.upload.max_upload_total_bytes);
    uploadSnapshot.value = serializeUploadState();
  } catch {
    error.value = "加载系统设置失败。";
  } finally {
    loaded.value = true;
    loading.value = false;
  }
}

async function saveUploadPolicy() {
  uploadSaving.value = true;
  error.value = "";
  message.value = "";
  form.upload.max_upload_total_bytes = toBytes(uploadSizeValue.value, uploadSizeUnit.value);

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

function serializeUploadState() {
  return JSON.stringify({
    max_upload_total_bytes: toBytes(uploadSizeValue.value, uploadSizeUnit.value),
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

const uploadDirty = computed(() => loaded.value && uploadSnapshot.value !== serializeUploadState());
const strictDirectoryInputKeyword = computed(() => {
  const current = normalizeManualBrowsePath(importCurrentPath.value);
  const manual = normalizeManualBrowsePath(manualBrowsePath.value);
  if (!current || !manual || manual === current) {
    return "";
  }
  const prefix = `${current}/`;
  if (!manual.startsWith(prefix)) {
    return "";
  }
  const remainder = manual.slice(prefix.length);
  if (!remainder || remainder.includes("/")) {
    return "";
  }
  return remainder.toLowerCase();
});
const filteredImportItems = computed(() => {
  const strictKeyword = strictDirectoryInputKeyword.value.trim();
  const looseKeyword = importFilter.value.trim().toLowerCase();

  return importItems.value.filter((item) => {
    const name = item.name.toLowerCase();
    const path = item.path.toLowerCase();

    if (strictKeyword && !name.startsWith(strictKeyword)) {
      return false;
    }

    if (looseKeyword && !name.includes(looseKeyword) && !path.includes(looseKeyword)) {
      return false;
    }

    return true;
  });
});
const importPathConflict = computed(() => {
  const selectedPath = normalizeManagedRootClientPath(importPath.value);
  if (!selectedPath) {
    return "";
  }

  for (const folder of managedFolders.value) {
    const existingPath = normalizeManagedRootClientPath(folder.sourcePath);
    if (!existingPath) {
      continue;
    }
    if (selectedPath === existingPath) {
      return "该目录已托管，请使用“重新扫描”。";
    }
    if (isManagedRootClientChild(selectedPath, existingPath)) {
      return "该目录位于已托管目录内，请对上级托管目录执行“重新扫描”。";
    }
    if (isManagedRootClientChild(existingPath, selectedPath)) {
      return "该目录包含已托管目录，不能重复导入父目录。";
    }
  }

  return "";
});

async function loadDirectories(path: string, options?: { silent?: boolean }) {
  importLoading.value = true;
  if (!options?.silent) {
    importError.value = "";
  }
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
    manualBrowsePath.value = withTrailingSlash(response.current_path);
    if (!importPath.value) {
      importPath.value = response.current_path;
    }
  } catch (err: unknown) {
    if (!options?.silent) {
      importError.value = readApiError(err, "加载目录浏览器失败。");
    }
  } finally {
    importLoading.value = false;
  }
}

async function loadManagedFolders() {
  managedFoldersLoading.value = true;
  managedFoldersError.value = "";
  try {
    const response = await httpClient.get<{ items: ManagedFolderNode[] }>("/admin/folders/tree");
    managedFolders.value = (response.items ?? []).map((item) => ({
      id: item.id,
      name: item.name,
      sourcePath: item.source_path,
    }));
  } catch (err: unknown) {
    managedFolders.value = [];
    managedFoldersError.value = readApiError(err, "加载已托管目录失败。");
  } finally {
    managedFoldersLoading.value = false;
  }
}

async function openDirectoryPicker() {
  directoryPickerOpen.value = true;
  importFilter.value = "";
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
  importFilter.value = "";
  await loadDirectories(path);
}

async function applyManualBrowsePath() {
  const nextPath = normalizeManualBrowsePath(manualBrowsePath.value);
  if (!nextPath) {
    return;
  }
  pendingImportPath.value = nextPath;
  importFilter.value = "";
  await loadDirectories(nextPath);
}

function updateManualBrowsePath(value: string) {
  manualBrowsePath.value = value;
}

function normalizeManualBrowsePath(value: string) {
  return value.trim().replace(/\/+$/, "");
}

function withTrailingSlash(value: string) {
  const trimmed = value.trim();
  if (!trimmed) {
    return "";
  }
  return trimmed.endsWith("/") ? trimmed : `${trimmed}/`;
}

async function importDirectory() {
  if (!importPath.value.trim()) {
    importError.value = "请先选择服务器目录。";
    return;
  }
  if (importPathConflict.value) {
    importError.value = importPathConflict.value;
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
    await loadManagedFolders();
  } catch (err: unknown) {
    importError.value = readApiError(err, "导入目录失败。");
  } finally {
    importLoading.value = false;
  }
}

async function rescanManagedFolder(folderID: string) {
  rescanningFolderID.value = folderID;
  rescanError.value = "";
  rescanMessage.value = "";
  try {
    const response = await httpClient.post<{
      added_folders: number;
      added_files: number;
      updated_folders: number;
      updated_files: number;
      deleted_folders: number;
      deleted_files: number;
    }>(`/admin/imports/local/${encodeURIComponent(folderID)}/rescan`);
    rescanMessage.value =
      `重新扫描完成：新增目录 ${response.added_folders} 个，新增文件 ${response.added_files} 个，` +
      `更新目录 ${response.updated_folders} 个，更新文件 ${response.updated_files} 个，` +
      `删除目录 ${response.deleted_folders} 个，删除文件 ${response.deleted_files} 个。`;
    await loadManagedFolders();
  } catch (err: unknown) {
    rescanError.value = readApiError(err, "重新扫描托管目录失败。");
  } finally {
    rescanningFolderID.value = "";
  }
}

function beginUnmanageManagedFolder(folderID: string) {
  unmanageError.value = "";
  unmanageMessage.value = "";
  unmanagePassword.value = "";
  unmanagingFolderID.value = folderID;
}

function cancelUnmanageManagedFolder() {
  unmanagingFolderID.value = "";
  unmanagePassword.value = "";
}

async function confirmUnmanageManagedFolder(folderID: string) {
  if (!unmanagePassword.value.trim()) {
    unmanageError.value = "请输入超级管理员密码。";
    return;
  }

  unmanageError.value = "";
  unmanageMessage.value = "";
  try {
    await httpClient.request(`/admin/imports/local/${encodeURIComponent(folderID)}`, {
      method: "DELETE",
      body: { password: unmanagePassword.value },
    });
    unmanageMessage.value = "已取消托管，并清理站内关联数据。";
    unmanagingFolderID.value = "";
    unmanagePassword.value = "";
    await loadManagedFolders();
  } catch (err: unknown) {
    unmanageError.value = readApiError(err, "取消托管目录失败。");
  }
}

function normalizeManagedRootClientPath(value: string) {
  return value.trim().replace(/\\/g, "/").replace(/\/+$/, "");
}

function isManagedRootClientChild(path: string, root: string) {
  return path !== root && path.startsWith(`${root}/`);
}
</script>

<template>
  <section class="space-y-4">
    <div>
      <h2 class="text-lg font-semibold tracking-tight text-slate-900">系统配置</h2>
    </div>

    <div v-if="!loaded && loading" class="text-sm text-slate-500">加载中…</div>

    <div v-else class="space-y-6">
      <SurfaceCard class="space-y-4">
        <div class="flex items-center justify-between gap-4">
          <div>
            <h3 class="text-lg font-semibold text-slate-900">当前已托管文件目录</h3>
          </div>
          <button type="button" class="btn-secondary" :disabled="managedFoldersLoading" @click="loadManagedFolders">
            {{ managedFoldersLoading ? "刷新中…" : "刷新" }}
          </button>
        </div>

        <div v-if="managedFoldersLoading" class="text-sm text-slate-500">正在加载托管目录…</div>
        <p v-else-if="managedFoldersError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ managedFoldersError }}</p>
        <div v-else-if="managedFolders.length === 0" class="panel-muted px-4 py-3 text-sm text-slate-500">
          暂无已托管目录。
        </div>
        <div v-else class="grid gap-3">
          <div
            v-for="folder in managedFolders"
            :key="folder.id"
            class="panel-muted px-4 py-3"
          >
            <div class="flex items-start justify-between gap-4">
              <div class="min-w-0">
                <p class="text-sm font-medium text-slate-900">{{ folder.name }}</p>
                <p class="mt-1 break-all text-sm text-slate-500">{{ folder.sourcePath || "未记录源目录" }}</p>
              </div>
              <div class="flex shrink-0 items-center gap-3">
                <button
                  type="button"
                  class="inline-flex h-11 items-center justify-center rounded-xl border border-slate-200 bg-white px-5 text-sm font-medium text-slate-700 transition hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-60"
                  :disabled="managedFoldersLoading || rescanningFolderID === folder.id"
                  @click="rescanManagedFolder(folder.id)"
                >
                  {{ rescanningFolderID === folder.id ? "扫描中…" : "重新扫描" }}
                </button>
                <button
                  type="button"
                  class="inline-flex h-11 items-center justify-center rounded-xl bg-rose-600 px-5 text-sm font-medium text-white transition hover:bg-rose-700 disabled:cursor-not-allowed disabled:opacity-60"
                  :disabled="managedFoldersLoading || rescanningFolderID === folder.id"
                  @click="beginUnmanageManagedFolder(folder.id)"
                >
                  取消托管
                </button>
              </div>
            </div>
            <div v-if="unmanagingFolderID === folder.id" class="mt-4 space-y-3 rounded-xl border border-rose-200 bg-white px-4 py-4">
              <p class="text-sm text-rose-700">该操作会取消此目录的托管并清理站内关联数据，原目录和文件会保留在原位置。</p>
              <input v-model="unmanagePassword" type="password" class="field" placeholder="输入 superadmin 密码确认取消托管" />
              <div class="flex items-center justify-end gap-3">
                <button type="button" class="inline-flex h-11 items-center justify-center rounded-xl border border-slate-200 bg-white px-5 text-sm font-medium text-slate-700 transition hover:bg-slate-100" @click="cancelUnmanageManagedFolder">取消</button>
                <button type="button" class="inline-flex h-11 items-center justify-center rounded-xl bg-rose-600 px-5 text-sm font-medium text-white transition hover:bg-rose-700" @click="confirmUnmanageManagedFolder(folder.id)">确认取消托管</button>
              </div>
            </div>
          </div>
        </div>
        <p v-if="rescanMessage" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ rescanMessage }}</p>
        <p v-if="rescanError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ rescanError }}</p>
        <p v-if="unmanageMessage" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ unmanageMessage }}</p>
        <p v-if="unmanageError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ unmanageError }}</p>
      </SurfaceCard>

      <div class="grid gap-6 xl:grid-cols-2">
      <form class="panel space-y-6 p-6" @submit.prevent="saveUploadPolicy">
        <div>
          <h3 class="text-lg font-semibold text-slate-900">上传设置</h3>
          <p class="mt-2 text-sm text-slate-500">访客只允许发起上传，所有公开上传都会先进入审核。单次提交里的文件总大小不能超过这里设置的上限。</p>
        </div>
        <div class="grid gap-4 md:grid-cols-[minmax(0,1fr)_140px]">
          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">单次提交总大小</label>
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
          <div class="panel-muted px-4 py-3">
            <p class="text-xs font-medium uppercase tracking-[0.12em] text-slate-400">已选目录</p>
            <p class="mt-2 break-all text-sm text-slate-700">{{ importPath || "尚未选择服务器目录" }}</p>
          </div>
          <p v-if="importPathConflict" class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-700">{{ importPathConflict }}</p>
        </div>
        <div class="space-y-3">
          <button type="button" class="btn-secondary w-full" :disabled="importLoading" @click="openDirectoryPicker">
            选择服务器目录
          </button>
          <button type="button" class="btn-primary w-full" :disabled="importLoading || !confirmedImportPath.trim() || !!importPathConflict" @click="importDirectory">
            {{ importLoading ? "导入中…" : "确认导入" }}
          </button>
        </div>
      </SurfaceCard>
      </div>
    </div>

    <p v-if="message" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ message }}</p>
    <p v-if="error" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ error }}</p>
    <p v-if="importMessage" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ importMessage }}</p>
    <p v-if="importError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ importError }}</p>

    <Teleport to="body">
    <Transition name="modal-shell">
    <div v-if="directoryPickerOpen" class="fixed inset-0 z-[120] overflow-hidden bg-slate-950/40 backdrop-blur-sm">
      <div class="flex h-full items-start justify-center px-4 py-6">
      <SurfaceCard class="modal-card w-full max-w-3xl overflow-hidden">
        <div class="flex items-start justify-between gap-4 border-b border-slate-200 pb-4">
          <div>
            <h3 class="text-lg font-semibold text-slate-900">选择服务器目录</h3>
            <p class="mt-1 text-sm text-slate-500">浏览服务器目录，确认后将当前目录作为导入源。</p>
          </div>
          <button type="button" class="btn-secondary" @pointerdown.prevent="closeDirectoryPicker">关闭</button>
        </div>

        <div class="mt-4 space-y-4">
          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">当前目录</label>
            <input
              :value="manualBrowsePath"
              type="text"
              class="field"
              placeholder="/Users/quan/Desktop/test/"
              @input="updateManualBrowsePath(($event.target as HTMLInputElement).value)"
              @keydown.enter.prevent="applyManualBrowsePath"
              @blur="applyManualBrowsePath"
            />
          </div>

          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">搜索子目录</label>
            <input v-model="importFilter" type="text" class="field" placeholder="输入关键字筛选目标目录" />
          </div>

          <div class="flex items-center justify-between gap-3">
            <button
              v-if="importParentPath"
              type="button"
              class="btn-secondary"
              @pointerdown.prevent="browseDirectory(importParentPath)"
            >
              上一级
            </button>
            <div v-else></div>
            <button
              type="button"
              class="btn-primary"
              :disabled="importLoading || !(pendingImportPath || importCurrentPath)"
              @pointerdown.prevent="selectCurrentDirectory"
            >
              选择当前目录
            </button>
          </div>

          <div class="max-h-[42vh] overflow-y-auto rounded-xl border border-slate-200 p-3">
            <div v-if="importLoading" class="py-6 text-center text-sm text-slate-500">目录加载中…</div>
            <div v-else-if="filteredImportItems.length === 0" class="py-6 text-center text-sm text-slate-500">没有匹配的目录，请继续输入或切换上级目录。</div>
            <div v-else class="space-y-2">
              <button
                v-for="item in filteredImportItems"
                :key="item.path"
                type="button"
                class="flex w-full items-center justify-between rounded-lg border border-slate-200 px-3 py-2.5 text-left text-sm text-slate-600 transition hover:bg-slate-50 hover:text-slate-900"
                @pointerdown.prevent="browseDirectory(item.path)"
              >
                <span>{{ item.name }}</span>
                <span class="text-xs text-slate-400">打开</span>
              </button>
            </div>
          </div>
        </div>
      </SurfaceCard>
      </div>
    </div>
    </Transition>
    </Teleport>
  </section>
</template>
