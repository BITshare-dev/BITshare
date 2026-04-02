<script setup lang="ts">
import { computed, onMounted, ref } from "vue";

import EmptyState from "../../components/ui/EmptyState.vue";
import PageHeader from "../../components/ui/PageHeader.vue";
import SurfaceCard from "../../components/ui/SurfaceCard.vue";
import { httpClient } from "../../lib/http/client";
import { readApiError } from "../../lib/http/helpers";

interface OperationLogItem {
  id: string;
  admin_id?: string | null;
  admin_name: string;
  action: string;
  target_type: string;
  target_id: string;
  detail: string;
  ip: string;
  created_at: string;
}

const items = ref<OperationLogItem[]>([]);
const loading = ref(false);
const loaded = ref(false);
const error = ref("");
const page = ref(1);
const pageSize = 20;
const total = ref(0);

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)));

onMounted(() => {
  void loadItems();
});

async function loadItems() {
  loading.value = true;
  error.value = "";
  try {
    const params = new URLSearchParams({
      page: String(page.value),
      page_size: String(pageSize),
    });

    const response = await httpClient.get<{ items: OperationLogItem[]; total: number }>(`/admin/operation-logs?${params.toString()}`);
    items.value = response.items ?? [];
    total.value = response.total ?? 0;
  } catch (err: unknown) {
    error.value = readApiError(err, "加载审计日志失败。");
  } finally {
    loaded.value = true;
    loading.value = false;
  }
}

function goToPage(next: number) {
  if (next < 1 || next > totalPages.value) return;
  page.value = next;
  void loadItems();
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function actionLabel(action: string) {
  const labels: Record<string, string> = {
    admin_created: "创建管理员",
    admin_updated: "修改管理员权限",
    admin_deleted: "删除管理员",
    admin_password_reset: "重置管理员密码",
    admin_password_changed: "修改自己的密码",
    admin_profile_updated: "更新账号资料",
    submission_approved: "通过上传审核",
    submission_rejected: "驳回上传审核",
    feedback_approved: "处理反馈",
    feedback_rejected: "驳回反馈",
    resource_updated: "更新资料信息",
    resource_deleted: "移入回收站",
    local_import: "导入本地目录",
    managed_directory_rescanned: "重新扫描托管目录",
    system_settings_updated: "更新系统设置",
    announcement_created: "创建公告",
    announcement_updated: "修改公告",
    announcement_deleted: "删除公告",
  };
  return labels[action] ?? action;
}

function targetTypeLabel(targetType: string) {
  const labels: Record<string, string> = {
    admin: "管理员",
    announcement: "公告",
    feedback: "反馈",
    file: "资料",
    folder: "目录",
    submission: "上传记录",
    system_setting: "系统设置",
  };
  return labels[targetType] ?? (targetType || "未知对象");
}

function parseDetail(detail: string) {
  const trimmed = detail?.trim();
  if (!trimmed) return null;
  try {
    return JSON.parse(trimmed) as Record<string, unknown>;
  } catch {
    return null;
  }
}

function summaryLabel(item: OperationLogItem) {
  const objectName = objectLabel(item);
  const detail = item.detail?.trim();
  const parsed = parseDetail(item.detail);

  switch (item.action) {
    case "admin_created":
      return `创建管理员账号：${detail || item.target_id}`;
    case "admin_updated":
      return `更新管理员 ${detail || item.target_id} 的状态或权限配置`;
    case "admin_deleted":
      return `删除管理员账号：${detail || item.target_id}`;
    case "admin_password_reset":
      return `重置管理员 ${detail || item.target_id} 的登录密码`;
    case "admin_password_changed":
      return "更新当前账号的登录密码";
    case "admin_profile_updated":
      return `更新当前账号资料${detail ? `，用户名更新为 ${detail}` : ""}`;
    case "submission_approved":
      return `通过上传审核${detail ? `：${detail}` : ""}`;
    case "submission_rejected":
      return `驳回上传审核${detail ? `：${detail}` : ""}`;
    case "feedback_approved":
      return `处理反馈：${objectName}${detail ? `，处理意见：${detail}` : ""}`;
    case "feedback_rejected":
      return `驳回反馈：${objectName}${detail ? `，处理说明：${detail}` : ""}`;
    case "resource_updated":
      return `更新资料信息${detail ? `：${detail}` : ""}`;
    case "resource_deleted":
      return `移入回收站${detail ? `：${detail}` : ""}`;
    case "local_import":
      if (parsed) {
        return `导入本地目录：${String(parsed.root_path ?? "-")}`;
      }
      return "导入本地目录";
    case "managed_directory_rescanned":
      if (parsed) {
        return `重新扫描托管目录：${String(parsed.root_path ?? "-")}`;
      }
      return "重新扫描托管目录";
    case "system_settings_updated":
      return "更新系统设置";
    case "announcement_created":
      return `发布公告${detail ? `：${detail}` : ""}`;
    case "announcement_updated":
      return `更新公告${detail ? `：${detail}` : ""}`;
    case "announcement_deleted":
      return `删除公告${detail ? `：${detail}` : ""}`;
    default:
      return `执行操作：${actionLabel(item.action)}，对象为 ${objectName}`;
  }
}

function objectLabel(item: OperationLogItem) {
  if (item.target_type === "system_setting") {
    return "系统设置";
  }
  return `${targetTypeLabel(item.target_type)} ${item.target_id || "-"}`;
}

function detailLines(item: OperationLogItem) {
  const detail = item.detail?.trim();
  const parsed = parseDetail(item.detail);

  if (item.action === "local_import" && parsed) {
    return [
      `导入目录：${String(parsed.root_path ?? "-")}`,
      `新增目录：${String(parsed.imported_folders ?? 0)} 个`,
      `新增文件：${String(parsed.imported_files ?? 0)} 个`,
      `跳过目录：${String(parsed.skipped_folders ?? 0)} 个`,
      `跳过文件：${String(parsed.skipped_files ?? 0)} 个`,
    ];
  }

  if (item.action === "managed_directory_rescanned" && parsed) {
    return [
      `扫描目录：${String(parsed.root_path ?? "-")}`,
      `新增目录：${String(parsed.added_folders ?? 0)} 个`,
      `新增文件：${String(parsed.added_files ?? 0)} 个`,
      `更新目录：${String(parsed.updated_folders ?? 0)} 个`,
      `更新文件：${String(parsed.updated_files ?? 0)} 个`,
      `删除目录：${String(parsed.deleted_folders ?? 0)} 个`,
      `删除文件：${String(parsed.deleted_files ?? 0)} 个`,
    ];
  }

  if (!detail) {
    return [`对象标识：${item.target_id || "-"}`];
  }

  if (detail.includes("->")) {
    const [before, after] = detail.split("->").map((part) => part.trim());
    return [`变更前：${before || "-"}`, `变更后：${after || "-"}`];
  }

  if (detail.includes(",")) {
    return detail.split(",").map((entry) => entry.trim()).filter(Boolean).map((entry) => `内容：${entry}`);
  }

  return [`内容：${detail}`];
}

function actorLabel(item: OperationLogItem) {
  return item.admin_name?.trim() || "系统";
}

function detailSummary(item: OperationLogItem) {
  return detailLines(item).join("；");
}
</script>

<template>
  <section class="space-y-4">
    <PageHeader
      eyebrow="LOGS"
      title="操作记录"
    />

    <p v-if="error" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ error }}</p>
    <p v-else-if="!loaded && loading" class="text-sm text-slate-500">加载中…</p>

    <section class="space-y-1">
      <p v-if="loading && loaded" class="text-sm text-slate-500">正在刷新日志…</p>

      <SurfaceCard v-for="item in items" :key="item.id">
        <div class="grid gap-x-3 gap-y-1 text-sm text-slate-600 xl:grid-cols-[minmax(0,1.7fr)_108px_210px_minmax(0,1.15fr)_168px]">
          <div class="min-w-0 space-y-1">
            <div class="flex flex-wrap items-center gap-1.5">
              <span class="inline-flex rounded-md bg-slate-100 px-1.5 py-0.5 text-[11px] leading-4 text-slate-700">{{ actionLabel(item.action) }}</span>
              <span class="inline-flex rounded-md bg-slate-50 px-1.5 py-0.5 text-[11px] leading-4 text-slate-500">{{ targetTypeLabel(item.target_type) }}</span>
            </div>
            <p class="min-w-0 truncate font-medium leading-5 text-slate-900">{{ summaryLabel(item) }}</p>
          </div>

          <div class="space-y-0.5 xl:self-center">
            <p class="text-[11px] leading-4 text-slate-400">操作人</p>
            <p class="truncate leading-5">{{ actorLabel(item) }}</p>
          </div>
          <div class="space-y-0.5 xl:self-center">
            <p class="text-[11px] leading-4 text-slate-400">对象</p>
            <p class="truncate leading-5">{{ objectLabel(item) }}</p>
          </div>
          <div class="space-y-0.5 xl:self-center">
            <p class="text-[11px] leading-4 text-slate-400">详情</p>
            <p class="truncate leading-5">{{ detailSummary(item) }}</p>
          </div>
          <div class="space-y-0.5 text-left xl:self-center xl:text-right">
            <p class="text-[11px] leading-4 text-slate-400">时间</p>
            <p class="leading-5 text-slate-700">{{ formatDate(item.created_at) }}</p>
            <p class="text-[11px] leading-4 text-slate-400">IP：{{ item.ip || "-" }}</p>
          </div>
        </div>
      </SurfaceCard>

      <EmptyState v-if="!loading && items.length === 0" title="暂无操作记录" description="当前尚未写入后台管理操作记录，请在后续操作后重新查看。" />

      <div v-if="totalPages > 1" class="mt-6 flex items-center justify-center gap-3">
        <button class="btn-secondary" :disabled="page <= 1" @click="goToPage(page - 1)">上一页</button>
        <span class="text-sm text-slate-500">{{ page }} / {{ totalPages }}</span>
        <button class="btn-secondary" :disabled="page >= totalPages" @click="goToPage(page + 1)">下一页</button>
      </div>
    </section>
  </section>
</template>
