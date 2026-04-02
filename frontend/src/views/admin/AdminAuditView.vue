<script setup lang="ts">
import { onMounted, ref } from "vue";

import EmptyState from "../../components/ui/EmptyState.vue";
import PageHeader from "../../components/ui/PageHeader.vue";
import SurfaceCard from "../../components/ui/SurfaceCard.vue";
import { httpClient } from "../../lib/http/client";
import { readApiError } from "../../lib/http/helpers";
import { useSessionStore } from "../../stores/session";

interface PendingSubmissionItem {
  submission_id: string;
  receipt_code: string;
  name: string;
  description: string;
  relative_path: string;
  review_reason: string;
  uploaded_at: string;
  size: number;
  mime_type: string;
}

interface FeedbackItem {
  id: string;
  receipt_code: string;
  file_id?: string | null;
  folder_id?: string | null;
  target_name: string;
  target_path: string;
  target_type: "file" | "folder";
  description: string;
  reporter_ip: string;
  status: string;
  created_at: string;
}

const sessionStore = useSessionStore();

const submissions = ref<PendingSubmissionItem[]>([]);
const submissionsLoading = ref(false);
const submissionsLoaded = ref(false);
const submissionsError = ref("");
const submissionActionMessage = ref("");
const submissionActionError = ref("");
const submissionRejectTarget = ref<PendingSubmissionItem | null>(null);
const submissionReviewReason = ref("");
const submissionRejectSubmitting = ref(false);

const feedbackItems = ref<FeedbackItem[]>([]);
const feedbackLoading = ref(false);
const feedbackLoaded = ref(false);
const feedbackError = ref("");
const feedbackActionMessage = ref("");
const feedbackActionError = ref("");
const feedbackReviewTarget = ref<FeedbackItem | null>(null);
const feedbackReviewMode = ref<"approve" | "reject">("approve");
const feedbackReviewReason = ref("");
const feedbackReviewSubmitting = ref(false);

onMounted(() => {
  if (sessionStore.hasPermission("submission_moderation")) {
    void loadSubmissions();
  }
  if (sessionStore.hasPermission("resource_moderation")) {
    void loadFeedback();
  }
});

async function loadSubmissions() {
  submissionsLoading.value = true;
  submissionsError.value = "";
  try {
    const response = await httpClient.get<{ items: PendingSubmissionItem[] }>("/admin/submissions/pending");
    submissions.value = response.items ?? [];
  } catch (err: unknown) {
    submissionsError.value = readApiError(err, "加载上传审核列表失败。");
  } finally {
    submissionsLoaded.value = true;
    submissionsLoading.value = false;
  }
}

async function approveSubmission(item: PendingSubmissionItem) {
  submissionActionMessage.value = "";
  submissionActionError.value = "";
  try {
    await httpClient.post(`/admin/submissions/${item.submission_id}/approve`);
    submissionActionMessage.value = `《${item.name}》已审核通过。`;
    await loadSubmissions();
    notifyPendingAuditChanged();
  } catch (err: unknown) {
    submissionActionError.value = readApiError(err, "审核通过失败。");
  }
}

function openRejectSubmissionDialog(item: PendingSubmissionItem) {
  submissionRejectTarget.value = item;
  submissionReviewReason.value = "";
}

function closeRejectSubmissionDialog() {
  submissionRejectTarget.value = null;
  submissionReviewReason.value = "";
  submissionRejectSubmitting.value = false;
}

async function rejectSubmission() {
  if (!submissionRejectTarget.value) {
    return;
  }
  if (!submissionReviewReason.value.trim()) {
    submissionActionError.value = "请输入驳回原因。";
    return;
  }
  submissionActionMessage.value = "";
  submissionActionError.value = "";
  submissionRejectSubmitting.value = true;
  try {
    await httpClient.post(`/admin/submissions/${submissionRejectTarget.value.submission_id}/reject`, {
      review_reason: submissionReviewReason.value.trim(),
    });
    submissionActionMessage.value = `《${submissionRejectTarget.value.name}》已驳回。`;
    await loadSubmissions();
    notifyPendingAuditChanged();
    closeRejectSubmissionDialog();
  } catch (err: unknown) {
    submissionActionError.value = readApiError(err, "驳回失败。");
  } finally {
    submissionRejectSubmitting.value = false;
  }
}

async function loadFeedback() {
  feedbackLoading.value = true;
  feedbackError.value = "";
  try {
    const response = await httpClient.get<{ items: FeedbackItem[] }>("/admin/feedback");
    feedbackItems.value = response.items ?? [];
  } catch (err: unknown) {
    feedbackError.value = readApiError(err, "加载反馈列表失败。");
  } finally {
    feedbackLoaded.value = true;
    feedbackLoading.value = false;
  }
}

function openApproveFeedbackDialog(item: FeedbackItem) {
  feedbackReviewTarget.value = item;
  feedbackReviewMode.value = "approve";
  feedbackReviewReason.value = "";
}

function openRejectFeedbackDialog(item: FeedbackItem) {
  feedbackReviewTarget.value = item;
  feedbackReviewMode.value = "reject";
  feedbackReviewReason.value = "";
}

function closeFeedbackReviewDialog() {
  feedbackReviewTarget.value = null;
  feedbackReviewReason.value = "";
  feedbackReviewSubmitting.value = false;
}

async function submitFeedbackReview() {
  if (!feedbackReviewTarget.value) return;
  if (feedbackReviewMode.value === "reject" && !feedbackReviewReason.value.trim()) {
    feedbackActionError.value = "请输入驳回说明。";
    return;
  }

  feedbackActionMessage.value = "";
  feedbackActionError.value = "";
  feedbackReviewSubmitting.value = true;
  try {
    await httpClient.post(
      `/admin/feedback/${feedbackReviewTarget.value.id}/${feedbackReviewMode.value}`,
      { review_reason: feedbackReviewReason.value.trim() },
    );
    feedbackActionMessage.value = feedbackReviewMode.value === "approve" ? "反馈已处理。" : "反馈已驳回。";
    await loadFeedback();
    notifyPendingAuditChanged();
    closeFeedbackReviewDialog();
  } catch (err: unknown) {
    feedbackActionError.value = readApiError(err, "操作失败，请重试。");
  } finally {
    feedbackReviewSubmitting.value = false;
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

function notifyPendingAuditChanged() {
  window.dispatchEvent(new Event("admin-pending-audit-refresh"));
}
</script>

<template>
  <section class="space-y-8">
    <PageHeader
      eyebrow="Audit"
      title="审核"
    />

    <section class="space-y-6">
      <SurfaceCard class="space-y-5">
        <div class="flex items-start justify-between gap-4">
          <div>
            <h2 class="text-lg font-semibold text-slate-900">上传审核</h2>
          </div>
          <button v-if="sessionStore.hasPermission('submission_moderation')" class="btn-secondary" @click="loadSubmissions">刷新</button>
        </div>

        <p v-if="submissionActionMessage" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ submissionActionMessage }}</p>
        <p v-if="submissionActionError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ submissionActionError }}</p>
        <p v-if="submissionsError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ submissionsError }}</p>

        <div v-if="!sessionStore.hasPermission('submission_moderation')" class="text-sm text-slate-500">当前账号没有上传审核权限。</div>
        <div v-else-if="!submissionsLoaded && submissionsLoading" class="text-sm text-slate-500">加载中…</div>
        <div v-else class="space-y-4">
          <div v-for="item in submissions" :key="item.submission_id" class="rounded-xl border border-slate-200 p-4">
            <div class="flex flex-wrap items-start justify-between gap-4">
              <div class="space-y-2">
                <div class="flex flex-wrap items-center gap-2">
                  <h3 class="text-base font-semibold text-slate-900">{{ item.name }}</h3>
                  <span class="rounded-md bg-slate-100 px-2.5 py-1 text-xs font-medium text-slate-600">{{ item.mime_type }}</span>
                </div>
                <p class="text-sm text-slate-500">{{ item.name }} · {{ formatSize(item.size) }}</p>
                <p v-if="item.relative_path" class="text-sm text-slate-500">目录结构：{{ item.relative_path }}</p>
                <p class="text-sm text-slate-500">回执码：{{ item.receipt_code }} · {{ formatDate(item.uploaded_at) }}</p>
                <p v-if="item.description" class="text-sm leading-6 text-slate-600">{{ item.description }}</p>
              </div>
              <div class="flex gap-2">
                <button class="btn-primary" @click="approveSubmission(item)">通过</button>
                <button class="btn-danger" @click="openRejectSubmissionDialog(item)">驳回</button>
              </div>
            </div>
          </div>
          <EmptyState v-if="!submissionsLoading && submissions.length === 0" title="当前没有待审核资料" />
        </div>
      </SurfaceCard>

      <SurfaceCard class="space-y-5">
        <div class="flex items-start justify-between gap-4">
          <div>
            <h2 class="text-lg font-semibold text-slate-900">用户反馈</h2>
          </div>
          <button v-if="sessionStore.hasPermission('resource_moderation')" class="btn-secondary" @click="loadFeedback">刷新</button>
        </div>

        <p v-if="feedbackActionMessage" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ feedbackActionMessage }}</p>
        <p v-if="feedbackActionError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ feedbackActionError }}</p>
        <p v-if="feedbackError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ feedbackError }}</p>

        <div v-if="!sessionStore.hasPermission('resource_moderation')" class="text-sm text-slate-500">当前账号没有资料管理权限。</div>
        <div v-else-if="!feedbackLoaded && feedbackLoading" class="text-sm text-slate-500">加载中…</div>
        <div v-else class="space-y-4">
          <div v-for="item in feedbackItems" :key="item.id" class="rounded-xl border border-slate-200 p-4">
            <div class="flex flex-wrap items-start justify-between gap-4">
              <div class="min-w-0 flex-1">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="rounded-lg bg-slate-100 px-2.5 py-1 text-xs text-slate-600">{{ item.target_type === "file" ? "文件" : "文件夹" }}</span>
                  <h3 class="text-base font-semibold text-slate-900">{{ item.target_name }}</h3>
                </div>
                <div class="mt-3 flex flex-wrap gap-3 text-sm text-slate-500">
                  <span>反馈时间：{{ formatDate(item.created_at) }}</span>
                  <span>回执码：{{ item.receipt_code }}</span>
                  <span>IP：{{ item.reporter_ip }}</span>
                </div>
                <p v-if="item.target_path" class="mt-3 text-sm text-slate-500 break-all">目标路径：{{ item.target_path }}</p>
                <p v-if="item.description" class="mt-4 rounded-xl bg-slate-50 px-4 py-3 text-sm leading-6 text-slate-600">{{ item.description }}</p>
              </div>
              <div class="flex shrink-0 flex-col gap-2">
                <button class="btn-primary" @click="openApproveFeedbackDialog(item)">已处理</button>
                <button class="btn-secondary" @click="openRejectFeedbackDialog(item)">驳回反馈</button>
              </div>
            </div>
          </div>
          <EmptyState v-if="!feedbackLoading && feedbackItems.length === 0" title="当前没有待处理反馈" />
        </div>
      </SurfaceCard>
    </section>
  </section>

  <Teleport to="body">
    <Transition name="modal-shell">
    <div v-if="submissionRejectTarget" class="fixed inset-0 z-[120] flex items-center justify-center bg-slate-950/30 px-4">
      <div class="modal-card w-full max-w-lg rounded-2xl bg-white p-6 shadow-xl">
        <div class="border-b border-slate-200 pb-4">
          <h3 class="text-lg font-semibold text-slate-900">驳回上传</h3>
          <p class="mt-2 text-sm leading-6 text-slate-500">填写驳回原因后，用户可在回执查询页看到驳回说明。</p>
        </div>
        <div class="mt-5 space-y-4">
          <div class="rounded-xl bg-slate-50 px-4 py-3 text-sm text-slate-600">
            <p class="font-medium text-slate-900">{{ submissionRejectTarget.name }}</p>
            <p class="mt-1">{{ submissionRejectTarget.name }} · {{ formatSize(submissionRejectTarget.size) }}</p>
            <p v-if="submissionRejectTarget.relative_path" class="mt-1">目录结构：{{ submissionRejectTarget.relative_path }}</p>
          </div>
          <textarea
            v-model="submissionReviewReason"
            rows="4"
            class="field-area"
            placeholder="例如：资料内容不完整 / 文件命名不规范 / 与当前目录主题不符"
          />
          <div class="flex justify-end gap-3 border-t border-slate-200 pt-4">
            <button type="button" class="btn-secondary" @click="closeRejectSubmissionDialog">取消</button>
            <button type="button" class="btn-danger" :disabled="submissionRejectSubmitting" @click="rejectSubmission">
              {{ submissionRejectSubmitting ? "提交中…" : "确认驳回" }}
            </button>
          </div>
        </div>
      </div>
    </div>
    </Transition>
  </Teleport>

  <Teleport to="body">
    <Transition name="modal-shell">
    <div v-if="feedbackReviewTarget" class="fixed inset-0 z-[120] flex items-center justify-center bg-slate-950/30 px-4">
      <div class="modal-card w-full max-w-lg rounded-2xl bg-white p-6 shadow-xl">
        <div class="border-b border-slate-200 pb-4">
          <h3 class="text-lg font-semibold text-slate-900">{{ feedbackReviewMode === "approve" ? "处理反馈" : "驳回反馈" }}</h3>
          <p class="mt-2 text-sm leading-6 text-slate-500">
            {{ feedbackReviewMode === "approve" ? "填写处理意见，用户可在回执查询页看到处理结果。" : "填写驳回说明，用户可在回执查询页看到驳回原因。" }}
          </p>
        </div>
        <div class="mt-5 space-y-4">
          <div class="rounded-xl bg-slate-50 px-4 py-3 text-sm text-slate-600">
            <p class="font-medium text-slate-900">{{ feedbackReviewTarget.target_name }}</p>
            <p v-if="feedbackReviewTarget.target_path" class="mt-1 break-all">{{ feedbackReviewTarget.target_path }}</p>
          </div>
          <textarea
            v-model="feedbackReviewReason"
            rows="4"
            class="field-area"
            :placeholder="feedbackReviewMode === 'approve' ? '例如：已修正资料内容 / 已补充缺失文件 / 已更新简介说明' : '例如：经核实资料内容无误，反馈不成立'"
          />
          <div class="flex justify-end gap-3 border-t border-slate-200 pt-4">
            <button type="button" class="btn-secondary" @click="closeFeedbackReviewDialog">取消</button>
            <button
              type="button"
              :class="feedbackReviewMode === 'approve' ? 'btn-primary' : 'btn-danger'"
              :disabled="feedbackReviewSubmitting"
              @click="submitFeedbackReview"
            >
              {{ feedbackReviewSubmitting ? "提交中…" : feedbackReviewMode === "approve" ? "确认已处理" : "确认驳回" }}
            </button>
          </div>
        </div>
      </div>
    </div>
    </Transition>
  </Teleport>
</template>
