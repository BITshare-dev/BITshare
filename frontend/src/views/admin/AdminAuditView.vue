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
  title: string;
  description: string;
  uploaded_at: string;
  file_name: string;
  file_size: number;
  file_mime_type: string;
}

interface PendingReportItem {
  id: string;
  target_name: string;
  target_type: "file" | "folder";
  reason: string;
  reason_label: string;
  description: string;
  reporter_ip: string;
  created_at: string;
}

const sessionStore = useSessionStore();

const submissions = ref<PendingSubmissionItem[]>([]);
const submissionsLoading = ref(false);
const submissionsLoaded = ref(false);
const submissionsError = ref("");
const submissionActionMessage = ref("");
const submissionActionError = ref("");

const reports = ref<PendingReportItem[]>([]);
const reportsLoading = ref(false);
const reportsLoaded = ref(false);
const reportsError = ref("");
const reportActionMessage = ref("");
const reportActionError = ref("");

onMounted(() => {
  if (sessionStore.hasPermission("review_submissions")) {
    void loadSubmissions();
  }
  if (sessionStore.hasPermission("review_reports")) {
    void loadReports();
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
    submissionActionMessage.value = `《${item.title}》已审核通过。`;
    await loadSubmissions();
  } catch (err: unknown) {
    submissionActionError.value = readApiError(err, "审核通过失败。");
  }
}

async function rejectSubmission(item: PendingSubmissionItem) {
  const rejectReason = window.prompt("请输入驳回原因：");
  if (!rejectReason) return;
  submissionActionMessage.value = "";
  submissionActionError.value = "";
  try {
    await httpClient.post(`/admin/submissions/${item.submission_id}/reject`, {
      reject_reason: rejectReason,
    });
    submissionActionMessage.value = `《${item.title}》已驳回。`;
    await loadSubmissions();
  } catch (err: unknown) {
    submissionActionError.value = readApiError(err, "驳回失败。");
  }
}

async function loadReports() {
  reportsLoading.value = true;
  reportsError.value = "";
  try {
    const response = await httpClient.get<{ items: PendingReportItem[] }>("/admin/reports/pending");
    reports.value = response.items ?? [];
  } catch (err: unknown) {
    reportsError.value = readApiError(err, "加载举报审核列表失败。");
  } finally {
    reportsLoaded.value = true;
    reportsLoading.value = false;
  }
}

async function approveReport(reportId: string) {
  const reviewReason = window.prompt("确认举报成立，资源将被下架。请输入处理说明（可选）：");
  if (reviewReason === null) return;
  reportActionError.value = "";
  reportActionMessage.value = "";
  try {
    await httpClient.post(`/admin/reports/${reportId}/approve`, { review_reason: reviewReason });
    reportActionMessage.value = "举报已处理，目标资源已下架。";
    await loadReports();
  } catch (err: unknown) {
    reportActionError.value = readApiError(err, "操作失败，请重试。");
  }
}

async function rejectReport(reportId: string) {
  const reviewReason = window.prompt("驳回举报，资源保持可见。请输入驳回说明（可选）：");
  if (reviewReason === null) return;
  reportActionError.value = "";
  reportActionMessage.value = "";
  try {
    await httpClient.post(`/admin/reports/${reportId}/reject`, { review_reason: reviewReason });
    reportActionMessage.value = "举报已驳回，资源保持公开。";
    await loadReports();
  } catch (err: unknown) {
    reportActionError.value = readApiError(err, "操作失败，请重试。");
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
          <button v-if="sessionStore.hasPermission('review_submissions')" class="btn-secondary" @click="loadSubmissions">刷新</button>
        </div>

        <p v-if="submissionActionMessage" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ submissionActionMessage }}</p>
        <p v-if="submissionActionError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ submissionActionError }}</p>
        <p v-if="submissionsError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ submissionsError }}</p>

        <div v-if="!sessionStore.hasPermission('review_submissions')" class="text-sm text-slate-500">当前账号没有上传审核权限。</div>
        <div v-else-if="!submissionsLoaded && submissionsLoading" class="text-sm text-slate-500">加载中…</div>
        <div v-else class="space-y-4">
          <div v-for="item in submissions" :key="item.submission_id" class="rounded-xl border border-slate-200 p-4">
            <div class="flex flex-wrap items-start justify-between gap-4">
              <div class="space-y-2">
                <div class="flex flex-wrap items-center gap-2">
                  <h3 class="text-base font-semibold text-slate-900">{{ item.title }}</h3>
                  <span class="rounded-md bg-slate-100 px-2.5 py-1 text-xs font-medium text-slate-600">{{ item.file_mime_type }}</span>
                </div>
                <p class="text-sm text-slate-500">{{ item.file_name }} · {{ formatSize(item.file_size) }}</p>
                <p class="text-sm text-slate-500">回执码：{{ item.receipt_code }} · {{ formatDate(item.uploaded_at) }}</p>
                <p v-if="item.description" class="text-sm leading-6 text-slate-600">{{ item.description }}</p>
              </div>
              <div class="flex gap-2">
                <button class="btn-primary" @click="approveSubmission(item)">通过</button>
                <button class="btn-danger" @click="rejectSubmission(item)">驳回</button>
              </div>
            </div>
          </div>
          <EmptyState v-if="!submissionsLoading && submissions.length === 0" title="当前没有待审核资料" />
        </div>
      </SurfaceCard>

      <SurfaceCard class="space-y-5">
        <div class="flex items-start justify-between gap-4">
          <div>
            <h2 class="text-lg font-semibold text-slate-900">举报审核</h2>
          </div>
          <button v-if="sessionStore.hasPermission('review_reports')" class="btn-secondary" @click="loadReports">刷新</button>
        </div>

        <p v-if="reportActionMessage" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ reportActionMessage }}</p>
        <p v-if="reportActionError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ reportActionError }}</p>
        <p v-if="reportsError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ reportsError }}</p>

        <div v-if="!sessionStore.hasPermission('review_reports')" class="text-sm text-slate-500">当前账号没有举报审核权限。</div>
        <div v-else-if="!reportsLoaded && reportsLoading" class="text-sm text-slate-500">加载中…</div>
        <div v-else class="space-y-4">
          <div v-for="report in reports" :key="report.id" class="rounded-xl border border-slate-200 p-4">
            <div class="flex flex-wrap items-start justify-between gap-4">
              <div class="min-w-0 flex-1">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="rounded-lg bg-slate-100 px-2.5 py-1 text-xs text-slate-600">{{ report.target_type === "file" ? "文件" : "文件夹" }}</span>
                  <h3 class="text-base font-semibold text-slate-900">{{ report.target_name }}</h3>
                </div>
                <div class="mt-3 flex flex-wrap gap-3 text-sm text-slate-500">
                  <span class="rounded-lg bg-rose-50 px-2.5 py-1 text-rose-700">{{ report.reason_label || report.reason }}</span>
                  <span>举报时间：{{ formatDate(report.created_at) }}</span>
                  <span>IP: {{ report.reporter_ip }}</span>
                </div>
                <p v-if="report.description" class="mt-4 rounded-xl bg-slate-50 px-4 py-3 text-sm leading-6 text-slate-600">{{ report.description }}</p>
              </div>
              <div class="flex shrink-0 flex-col gap-2">
                <button class="btn-danger" @click="approveReport(report.id)">确认下架</button>
                <button class="btn-secondary" @click="rejectReport(report.id)">驳回举报</button>
              </div>
            </div>
          </div>
          <EmptyState v-if="!reportsLoading && reports.length === 0" title="当前没有待处理举报" />
        </div>
      </SurfaceCard>
    </section>
  </section>
</template>
