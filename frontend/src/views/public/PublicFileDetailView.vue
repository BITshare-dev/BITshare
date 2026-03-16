<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";

import PageHeader from "../../components/ui/PageHeader.vue";
import SurfaceCard from "../../components/ui/SurfaceCard.vue";
import { HttpError, httpClient } from "../../lib/http/client";
import { readApiError } from "../../lib/http/helpers";

type PreviewKind = "none" | "pdf" | "image" | "text";

interface FileDetailResponse {
  id: string;
  title: string;
  description: string;
  original_name: string;
  mime_type: string;
  size: number;
  tags: string[];
  uploaded_at: string;
  download_count: number;
  preview_kind: PreviewKind;
  can_preview: boolean;
}

interface PublicPolicyResponse {
  guest: {
    allow_guest_edit_title: boolean;
    allow_guest_edit_tags: boolean;
    allow_guest_edit_description: boolean;
    allow_guest_resource_delete: boolean;
  };
}

const route = useRoute();
const detail = ref<FileDetailResponse | null>(null);
const loading = ref(false);
const error = ref("");
const message = ref("");
const saving = ref(false);
const deleting = ref(false);
const guestPolicy = ref<PublicPolicyResponse["guest"] | null>(null);
const editTitle = ref("");
const editDescription = ref("");
const editTags = ref("");
const textPreview = ref("");
const textPreviewLoading = ref(false);
const textPreviewError = ref("");

const fileID = computed(() => String(route.params.fileID ?? ""));
const previewURL = computed(() => `/api/public/files/${encodeURIComponent(fileID.value)}/preview`);
const downloadURL = computed(() => `/api/public/files/${encodeURIComponent(fileID.value)}/download`);

onMounted(() => {
  void Promise.all([loadDetail(), loadPolicy()]);
});

watch(fileID, () => {
  void Promise.all([loadDetail(), loadPolicy()]);
});

async function loadDetail() {
  loading.value = true;
  error.value = "";
  textPreview.value = "";
  textPreviewError.value = "";
  try {
    detail.value = await httpClient.get<FileDetailResponse>(`/public/files/${encodeURIComponent(fileID.value)}`);
    if (detail.value) {
      editTitle.value = detail.value.title;
      editDescription.value = detail.value.description;
      editTags.value = detail.value.tags.join(", ");
      await loadTextPreview(detail.value.preview_kind);
    }
  } catch (err: unknown) {
    if (err instanceof HttpError && err.status === 404) {
      error.value = "文件不存在或未公开。";
    } else {
      error.value = "加载文件详情失败。";
    }
  } finally {
    loading.value = false;
  }
}

async function loadTextPreview(kind: PreviewKind) {
  if (kind !== "text") {
    return;
  }

  textPreviewLoading.value = true;
  try {
    const response = await fetch(previewURL.value, { credentials: "include" });
    if (!response.ok) {
      throw new Error("preview request failed");
    }
    textPreview.value = await response.text();
  } catch {
    textPreviewError.value = "文本预览加载失败，请直接下载文件。";
  } finally {
    textPreviewLoading.value = false;
  }
}

async function loadPolicy() {
  try {
    const response = await httpClient.get<PublicPolicyResponse>("/public/system/policy");
    guestPolicy.value = response.guest;
  } catch {
    guestPolicy.value = null;
  }
}

const canEditTitle = computed(() => !!guestPolicy.value?.allow_guest_edit_title);
const canEditTags = computed(() => !!guestPolicy.value?.allow_guest_edit_tags);
const canEditDescription = computed(() => !!guestPolicy.value?.allow_guest_edit_description);
const canEdit = computed(() => canEditTitle.value || canEditTags.value || canEditDescription.value);
const canDelete = computed(() => !!guestPolicy.value?.allow_guest_resource_delete);

async function savePublicEdit() {
  if (!detail.value) return;
  saving.value = true;
  error.value = "";
  message.value = "";
  try {
    await httpClient.request(`/public/files/${encodeURIComponent(detail.value.id)}`, {
      method: "PUT",
      body: {
        title: editTitle.value,
        description: editDescription.value,
        tags: editTags.value.split(",").map((item) => item.trim()).filter(Boolean),
      },
    });
    message.value = "资料信息已更新。";
    await loadDetail();
  } catch (err: unknown) {
    error.value = readApiError(err, "更新资料失败。");
  } finally {
    saving.value = false;
  }
}

async function deletePublicFile() {
  if (!detail.value || !window.confirm("确认删除这个资料吗？")) return;
  deleting.value = true;
  error.value = "";
  message.value = "";
  try {
    await httpClient.request(`/public/files/${encodeURIComponent(detail.value.id)}`, { method: "DELETE" });
    message.value = "资料已删除。";
    detail.value = null;
  } catch (err: unknown) {
    error.value = readApiError(err, "删除资料失败。");
  } finally {
    deleting.value = false;
  }
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function formatSize(size: number) {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}
</script>

<template>
  <section class="app-container space-y-6 py-8 sm:py-10">
    <PageHeader eyebrow="File Detail" title="文件详情" description="展示资料基本信息、在线预览和访客可用的公开维护能力。" />

    <p v-if="loading" class="panel px-4 py-3 text-sm text-slate-500">加载中…</p>
    <p v-else-if="error" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ error }}</p>
    <p v-if="message" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ message }}</p>

    <template v-else-if="detail">
      <SurfaceCard>
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h3 class="text-2xl font-semibold text-slate-900">{{ detail.title }}</h3>
            <p class="mt-3 text-sm leading-6 text-slate-600">{{ detail.description || "暂无描述" }}</p>
          </div>
          <a
            :href="downloadURL"
            class="btn-primary"
          >
            下载文件
          </a>
        </div>

        <div class="mt-5 flex flex-wrap gap-2">
          <span
            v-for="tag in detail.tags"
            :key="tag"
            class="rounded-full bg-blue-100 px-3 py-1 text-xs font-medium text-blue-800"
          >
            {{ tag }}
          </span>
          <span v-if="detail.tags.length === 0" class="rounded-full bg-slate-100 px-3 py-1 text-xs text-slate-600">
            无 Tag
          </span>
        </div>

        <dl class="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          <div class="rounded-2xl bg-slate-50 px-4 py-4">
            <dt class="text-xs uppercase tracking-[0.16em] text-slate-500">上传时间</dt>
            <dd class="mt-2 text-sm font-semibold text-slate-900">{{ formatDate(detail.uploaded_at) }}</dd>
          </div>
          <div class="rounded-2xl bg-slate-50 px-4 py-4">
            <dt class="text-xs uppercase tracking-[0.16em] text-slate-500">下载次数</dt>
            <dd class="mt-2 text-sm font-semibold text-slate-900">{{ detail.download_count }}</dd>
          </div>
          <div class="rounded-2xl bg-slate-50 px-4 py-4">
            <dt class="text-xs uppercase tracking-[0.16em] text-slate-500">文件大小</dt>
            <dd class="mt-2 text-sm font-semibold text-slate-900">{{ formatSize(detail.size) }}</dd>
          </div>
          <div class="rounded-2xl bg-slate-50 px-4 py-4">
            <dt class="text-xs uppercase tracking-[0.16em] text-slate-500">文件类型</dt>
            <dd class="mt-2 text-sm font-semibold text-slate-900">{{ detail.original_name }}</dd>
          </div>
        </dl>
      </SurfaceCard>

      <SurfaceCard>
        <div class="flex items-center justify-between gap-4">
          <div>
            <p class="text-sm font-semibold uppercase tracking-[0.22em] text-blue-700">Preview</p>
            <h3 class="mt-2 text-2xl font-semibold text-slate-900">在线预览</h3>
          </div>
          <span class="rounded-full bg-slate-100 px-3 py-1 text-xs font-medium text-slate-600">
            {{ detail.preview_kind }}
          </span>
        </div>

        <div class="mt-6">
          <iframe
            v-if="detail.preview_kind === 'pdf'"
            :src="previewURL"
            class="h-[70vh] w-full rounded-2xl border border-slate-200"
          />
          <img
            v-else-if="detail.preview_kind === 'image'"
            :src="previewURL"
            :alt="detail.title"
            class="max-h-[70vh] w-full rounded-2xl border border-slate-200 object-contain"
          />
          <div
            v-else-if="detail.preview_kind === 'text'"
            class="min-h-[50vh] rounded-2xl border border-slate-200 bg-slate-50 p-5"
          >
            <p v-if="textPreviewLoading" class="text-sm text-slate-500">正在加载文本预览…</p>
            <p v-else-if="textPreviewError" class="text-sm text-rose-600">{{ textPreviewError }}</p>
            <pre v-else class="overflow-auto whitespace-pre-wrap break-words text-sm leading-6 text-slate-700">{{ textPreview }}</pre>
          </div>
          <div v-else class="rounded-2xl border border-dashed border-slate-300 bg-slate-50 px-6 py-10 text-sm text-slate-600">
            当前格式不支持在线预览，请直接下载文件。
          </div>
        </div>
      </SurfaceCard>

      <SurfaceCard v-if="canEdit || canDelete">
        <div class="flex items-center justify-between gap-4">
          <div>
            <p class="text-sm font-semibold uppercase tracking-[0.22em] text-blue-700">Guest Controls</p>
            <h3 class="mt-2 text-2xl font-semibold text-slate-900">公开资料维护</h3>
          </div>
          <div class="flex gap-2">
            <button
              v-if="canDelete"
              class="btn-danger"
              :disabled="deleting"
              @click="deletePublicFile"
            >
              {{ deleting ? "删除中…" : "删除资料" }}
            </button>
          </div>
        </div>

        <form v-if="canEdit" class="mt-6 space-y-4" @submit.prevent="savePublicEdit">
          <input v-if="canEditTitle" v-model="editTitle" class="field" />
          <textarea v-if="canEditDescription" v-model="editDescription" rows="4" class="field-area" />
          <input v-if="canEditTags" v-model="editTags" placeholder="Tag, 用逗号分隔" class="field" />
          <button type="submit" class="btn-primary" :disabled="saving">
            {{ saving ? "保存中…" : "保存修改" }}
          </button>
        </form>
      </SurfaceCard>
    </template>
  </section>
</template>
