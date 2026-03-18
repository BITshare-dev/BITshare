<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";

import EmptyState from "../../components/ui/EmptyState.vue";
import PageHeader from "../../components/ui/PageHeader.vue";
import SurfaceCard from "../../components/ui/SurfaceCard.vue";
import { httpClient } from "../../lib/http/client";
import { readApiError } from "../../lib/http/helpers";
import { useSessionStore } from "../../stores/session";

interface AdminItem {
  id: string;
  username: string;
  display_name: string;
  avatar_url: string;
  role: string;
  status: "active" | "disabled";
  permissions: string[];
  created_at: string;
  updated_at: string;
}

interface AdminListResponseItem extends Omit<AdminItem, "permissions"> {
  permissions: string[] | null;
}

interface CreatedAdminResponse {
  item: AdminItem;
  login_id: string;
  password: string;
  display_name: string;
}

interface AdminSnapshot {
  status: AdminItem["status"];
  permissions: string[];
  expandedPermissions: string[];
}

const sessionStore = useSessionStore();
const canManageAdmins = computed(() => sessionStore.hasPermission("manage_admins"));

const permissionOptions = [
  { value: "submission_moderation", label: "上传审核 / 免审上传" },
  { value: "resource_moderation", label: "反馈处理 / 编辑资料 / 删除资料" },
  { value: "announcements", label: "公告" },
];

const permissionMap: Record<string, string[]> = {
  submission_moderation: ["submission_moderation"],
  resource_moderation: ["resource_moderation"],
  announcements: ["announcements"],
};

const hiddenPermissions = ["manage_admins", "manage_system"];

const items = ref<AdminItem[]>([]);
const loading = ref(false);
const loaded = ref(false);
const error = ref("");
const message = ref("");
const form = reactive({
  permissions: [] as string[],
});
const creating = ref(false);
const createdCredentials = ref<CreatedAdminResponse | null>(null);
const deletingAdmin = ref<AdminItem | null>(null);
const resettingAdmin = ref<AdminItem | null>(null);
const statusConfirmAdmin = ref<AdminItem | null>(null);
const resetPasswordForm = reactive({
  password: "",
  confirmPassword: "",
});
const resetPasswordSaving = ref(false);
const statusSaving = ref(false);
const snapshots = ref<Record<string, AdminSnapshot>>({});

const canCreateAdmin = computed(() => form.permissions.length > 0 && !creating.value);
const visibleItems = computed(() => {
  return [...items.value].sort((left, right) => {
    if (left.id === sessionStore.adminId && right.id !== sessionStore.adminId) return -1;
    if (right.id === sessionStore.adminId && left.id !== sessionStore.adminId) return 1;
    return new Date(left.created_at).getTime() - new Date(right.created_at).getTime();
  });
});

onMounted(() => {
  void loadItems();
});

async function loadItems() {
  loading.value = true;
  error.value = "";
  message.value = "";
  try {
    const response = await httpClient.get<{ items: AdminListResponseItem[] }>("/admin/admins");
    const nextSnapshots: Record<string, AdminSnapshot> = {};
    items.value = (response.items ?? []).map((item) => {
      const normalizedPermissions = Array.isArray(item.permissions) ? item.permissions : [];
      const selectedPermissions = toPermissionSelection(normalizedPermissions);
      nextSnapshots[item.id] = {
        status: item.status,
        permissions: [...selectedPermissions],
        expandedPermissions: [...normalizedPermissions],
      };
      return {
        ...item,
        permissions: selectedPermissions,
      };
    });
    snapshots.value = nextSnapshots;
  } catch {
    error.value = "加载管理员列表失败。";
  } finally {
    loaded.value = true;
    loading.value = false;
  }
}

async function createAdmin() {
  error.value = "";
  message.value = "";
  creating.value = true;
  try {
    const response = await httpClient.post<CreatedAdminResponse>("/admin/admins", {
      permissions: expandPermissions(form.permissions),
    });
    form.permissions = [];
    createdCredentials.value = response;
    message.value = "管理员已创建。";
    await loadItems();
  } catch (err: unknown) {
    error.value = readApiError(err, "创建管理员失败。");
  } finally {
    creating.value = false;
  }
}

async function saveAdmin(item: AdminItem) {
  error.value = "";
  message.value = "";
  try {
    await httpClient.request(`/admin/admins/${item.id}`, {
      method: "PUT",
      body: {
        status: item.status,
        permissions: expandPermissions(item.permissions, snapshots.value[item.id]?.expandedPermissions ?? []),
      },
    });
    message.value = `管理员 ${item.display_name} 已更新。`;
    await loadItems();
    return true;
  } catch (err: unknown) {
    error.value = readApiError(err, "更新管理员失败。");
    return false;
  }
}

async function resetPassword(item: AdminItem) {
  resetPasswordForm.password = "";
  resetPasswordForm.confirmPassword = "";
  resettingAdmin.value = item;
}

async function confirmResetPassword() {
  if (!resettingAdmin.value) return;
  error.value = "";
  message.value = "";
  if (resetPasswordForm.password.length < 8) {
    error.value = "新密码至少 8 位。";
    return;
  }
  if (resetPasswordForm.password !== resetPasswordForm.confirmPassword) {
    error.value = "两次输入的密码不一致。";
    return;
  }
  resetPasswordSaving.value = true;
  try {
    await httpClient.post(`/admin/admins/${resettingAdmin.value.id}/reset-password`, { new_password: resetPasswordForm.password });
    message.value = `管理员 ${resettingAdmin.value.display_name} 的密码已重置。`;
    resettingAdmin.value = null;
  } catch (err: unknown) {
    error.value = readApiError(err, "重置密码失败。");
  } finally {
    resetPasswordSaving.value = false;
  }
}

async function deleteAdmin(item: AdminItem) {
  deletingAdmin.value = item;
}

async function confirmDeleteAdmin() {
  if (!deletingAdmin.value) return;
  error.value = "";
  message.value = "";
  try {
    await httpClient.request(`/admin/admins/${deletingAdmin.value.id}`, { method: "DELETE" });
    message.value = `管理员 ${deletingAdmin.value.display_name} 已删除。`;
    deletingAdmin.value = null;
    await loadItems();
  } catch (err: unknown) {
    error.value = readApiError(err, "删除管理员失败。");
  }
}

function togglePermission(item: AdminItem, permission: string) {
  if (item.permissions.includes(permission)) {
    item.permissions = item.permissions.filter((entry) => entry !== permission);
  } else {
    item.permissions = [...item.permissions, permission];
  }
}

function toPermissionSelection(permissions: string[]) {
  return permissionOptions
    .filter((option) => permissionMap[option.value].every((permission) => permissions.includes(permission)))
    .map((option) => option.value);
}

function expandPermissions(selected: string[], existing: string[] = []) {
  const expanded = new Set<string>(hiddenPermissions.filter((permission) => existing.includes(permission)));
  for (const key of selected) {
    for (const permission of permissionMap[key] ?? []) {
      expanded.add(permission);
    }
  }
  return [...expanded];
}

function hasSelectedPermission(item: AdminItem, permission: string) {
  return item.permissions.includes(permission);
}

function isAdminDirty(item: AdminItem) {
  const snapshot = snapshots.value[item.id];
  if (!snapshot) return false;
  const currentPermissions = [...item.permissions].sort().join(",");
  const originalPermissions = [...snapshot.permissions].sort().join(",");
  return item.status !== snapshot.status || currentPermissions !== originalPermissions;
}

function toggleAdminStatus(item: AdminItem) {
  statusConfirmAdmin.value = item;
}

function closeCreatedModal() {
  createdCredentials.value = null;
}

function closeDeleteModal() {
  deletingAdmin.value = null;
}

function closeResetPasswordModal() {
  resettingAdmin.value = null;
  resetPasswordForm.password = "";
  resetPasswordForm.confirmPassword = "";
}

async function confirmToggleStatus() {
  if (!statusConfirmAdmin.value) return;
  statusSaving.value = true;
  error.value = "";
  message.value = "";
  const nextStatus = statusConfirmAdmin.value.status === "active" ? "disabled" : "active";
  const previousStatus = statusConfirmAdmin.value.status;
  statusConfirmAdmin.value.status = nextStatus;
  try {
    const saved = await saveAdmin(statusConfirmAdmin.value);
    if (saved) {
      statusConfirmAdmin.value = null;
    } else {
      statusConfirmAdmin.value.status = previousStatus;
    }
  } finally {
    statusSaving.value = false;
  }
}

function closeStatusModal() {
  statusConfirmAdmin.value = null;
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
  }).format(new Date(value));
}
</script>

<template>
  <section class="space-y-8">
    <PageHeader
      eyebrow="PERMISSIONS"
      title="权限管理"
    />

    <section :class="canManageAdmins ? 'grid gap-6 xl:grid-cols-[420px_minmax(0,1fr)]' : 'space-y-6'">
      <SurfaceCard v-if="canManageAdmins">
        <h2 class="text-lg font-semibold text-slate-900">创建管理员</h2>
        <form class="mt-6 space-y-4" @submit.prevent="createAdmin">
          <div class="flex flex-wrap gap-2">
            <label
              v-for="permission in permissionOptions"
              :key="permission.value"
              class="rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-700"
            >
              <input v-model="form.permissions" :value="permission.value" type="checkbox" class="mr-2" />
              {{ permission.label }}
            </label>
          </div>
          <button type="submit" class="btn-primary" :disabled="!canCreateAdmin">
            {{ creating ? "创建中…" : "创建账号" }}
          </button>
        </form>
      </SurfaceCard>

      <SurfaceCard>
        <div class="flex items-center justify-between gap-4">
          <div>
            <h2 class="text-lg font-semibold text-slate-900">管理员列表</h2>
          </div>
          <button class="btn-secondary" @click="loadItems">刷新</button>
        </div>

        <div v-if="!loaded && loading" class="mt-4 text-sm text-slate-500">加载中…</div>
        <div v-else class="mt-6 space-y-4">
          <article v-for="item in visibleItems" :key="item.id" class="rounded-xl border border-slate-200 p-4">
            <div class="flex flex-wrap items-start justify-between gap-4">
              <div class="flex min-w-0 items-start gap-3">
                <div class="flex h-12 w-12 shrink-0 items-center justify-center overflow-hidden rounded-xl border border-slate-200 bg-slate-100 text-base font-semibold text-slate-700">
                  <img v-if="item.avatar_url" :src="item.avatar_url" alt="管理员头像" class="h-full w-full object-cover" />
                  <span v-else>{{ item.display_name.slice(0, 1).toUpperCase() || "A" }}</span>
                </div>
                <div class="min-w-0">
                  <div class="flex flex-wrap items-center gap-2">
                    <h3 class="text-sm font-semibold text-slate-900 sm:text-base">{{ item.display_name }}</h3>
                    <span v-if="item.id === sessionStore.adminId" class="rounded-lg bg-blue-50 px-2.5 py-1 text-xs text-blue-700">当前账号</span>
                    <span class="rounded-lg bg-slate-100 px-2.5 py-1 text-xs text-slate-600">{{ item.role }}</span>
                    <span class="rounded-lg bg-slate-100 px-2.5 py-1 text-xs text-slate-600">{{ item.status }}</span>
                  </div>
                  <p class="mt-1.5 text-xs text-slate-500 sm:text-sm">标示ID：{{ item.username }} · 创建时间：{{ formatDate(item.created_at) }}</p>
                </div>
              </div>
              <div v-if="canManageAdmins && item.role !== 'super_admin'" class="flex shrink-0 gap-1.5 self-start">
                <button
                  class="inline-flex h-8 items-center rounded-lg border border-slate-200 px-3 text-xs font-medium text-slate-600 transition hover:bg-slate-50 hover:text-slate-900 sm:text-sm"
                  @click="toggleAdminStatus(item)"
                >
                  {{ item.status === "active" ? "停用账号" : "重新启用" }}
                </button>
                <button class="inline-flex h-8 items-center rounded-lg border border-slate-200 px-3 text-xs font-medium text-slate-600 transition hover:bg-slate-50 hover:text-slate-900 sm:text-sm" @click="resetPassword(item)">重置密码</button>
                <button class="inline-flex h-8 items-center rounded-lg bg-rose-600 px-3 text-xs font-medium text-white transition hover:bg-rose-700 sm:text-sm" @click="deleteAdmin(item)">删除</button>
              </div>
            </div>

            <div v-if="canManageAdmins && item.role !== 'super_admin'" class="mt-4 space-y-4">
              <div class="flex items-start justify-between gap-4">
                <div class="flex flex-1 flex-wrap gap-2">
                  <button
                    v-for="permission in permissionOptions"
                    :key="permission.value"
                    type="button"
                    class="rounded-xl border px-3 py-1.5 text-[13px] transition"
                    :class="hasSelectedPermission(item, permission.value) ? 'border-blue-200 bg-blue-50 text-blue-700' : 'border-slate-200 bg-white text-slate-600 hover:bg-slate-50'"
                    @click="togglePermission(item, permission.value)"
                  >
                    {{ permission.label }}
                  </button>
                </div>
                <button class="btn-primary shrink-0" :disabled="!isAdminDirty(item)" @click="saveAdmin(item)">
                  保存
                </button>
              </div>
            </div>
          </article>

          <EmptyState v-if="visibleItems.length === 0" title="当前没有管理员数据" description="创建新的管理员账号后，这里会展示账号状态和权限。" />
        </div>
      </SurfaceCard>
    </section>

    <p v-if="message" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ message }}</p>
    <p v-if="error" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ error }}</p>

    <Transition name="modal-shell">
    <div v-if="createdCredentials" class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/30 px-4">
      <div class="modal-card w-full max-w-md rounded-2xl bg-white p-6 shadow-xl">
        <div>
          <div>
            <h3 class="text-lg font-semibold text-slate-900">管理员已创建</h3>
            <p class="mt-1 text-sm text-slate-500">请保存下面的初始登录信息。窗口关闭后不会再次显示。</p>
          </div>
        </div>

        <div class="mt-6 space-y-4">
          <div class="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3">
            <p class="text-xs font-medium uppercase tracking-wide text-slate-400">标示ID</p>
            <p class="mt-1 text-base font-semibold text-slate-900">{{ createdCredentials.login_id }}</p>
          </div>
          <div class="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3">
            <p class="text-xs font-medium uppercase tracking-wide text-slate-400">用户名</p>
            <p class="mt-1 text-base font-semibold text-slate-900">{{ createdCredentials.display_name }}</p>
          </div>
          <div class="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3">
            <p class="text-xs font-medium uppercase tracking-wide text-slate-400">初始密码</p>
            <p class="mt-1 text-base font-semibold text-slate-900">{{ createdCredentials.password }}</p>
          </div>
        </div>

        <div class="mt-6 flex justify-end">
          <button type="button" class="btn-primary" @click="closeCreatedModal">关闭</button>
        </div>
      </div>
    </div>
    </Transition>

    <Transition name="modal-shell">
    <div v-if="deletingAdmin" class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/30 px-4">
      <div class="modal-card w-full max-w-md rounded-2xl bg-white p-6 shadow-xl">
        <div>
          <h3 class="text-lg font-semibold text-slate-900">确认删除管理员</h3>
          <p class="mt-2 text-sm leading-6 text-slate-500">
            删除后将清除该管理员账号及其会话，无法恢复。确认删除
            <span class="font-medium text-slate-900">{{ deletingAdmin.display_name }}</span>
            吗？
          </p>
        </div>
        <div class="mt-6 flex justify-end gap-3">
          <button type="button" class="btn-secondary" @click="closeDeleteModal">取消</button>
          <button type="button" class="inline-flex h-11 items-center rounded-xl bg-rose-600 px-5 text-sm font-medium text-white transition hover:bg-rose-700" @click="confirmDeleteAdmin">
            确认删除
          </button>
        </div>
      </div>
    </div>
    </Transition>

    <Transition name="modal-shell">
    <div v-if="resettingAdmin" class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/30 px-4">
      <div class="modal-card w-full max-w-md rounded-2xl bg-white p-6 shadow-xl">
        <div>
          <h3 class="text-lg font-semibold text-slate-900">重置密码</h3>
          <p class="mt-2 text-sm leading-6 text-slate-500">
            为 <span class="font-medium text-slate-900">{{ resettingAdmin.display_name }}</span> 设置新的登录密码。
          </p>
        </div>
        <div class="mt-6 space-y-4">
          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">新密码</label>
            <input v-model="resetPasswordForm.password" type="password" class="field" placeholder="至少 8 位" />
          </div>
          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">确认新密码</label>
            <input v-model="resetPasswordForm.confirmPassword" type="password" class="field" placeholder="再次输入新密码" />
          </div>
        </div>
        <div class="mt-6 flex justify-end gap-3">
          <button type="button" class="btn-secondary" @click="closeResetPasswordModal">取消</button>
          <button type="button" class="btn-primary" :disabled="resetPasswordSaving" @click="confirmResetPassword">
            {{ resetPasswordSaving ? "处理中…" : "确认重置" }}
          </button>
        </div>
      </div>
    </div>
    </Transition>

    <Transition name="modal-shell">
    <div v-if="statusConfirmAdmin" class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/30 px-4">
      <div class="modal-card w-full max-w-md rounded-2xl bg-white p-6 shadow-xl">
        <div>
          <h3 class="text-lg font-semibold text-slate-900">{{ statusConfirmAdmin.status === "active" ? "停用账号" : "重新启用" }}</h3>
          <p class="mt-2 text-sm leading-6 text-slate-500">
            确认要{{ statusConfirmAdmin.status === "active" ? "停用" : "重新启用" }}
            <span class="font-medium text-slate-900">{{ statusConfirmAdmin.display_name }}</span> 吗？
          </p>
        </div>
        <div class="mt-6 flex justify-end gap-3">
          <button type="button" class="btn-secondary" @click="closeStatusModal">取消</button>
          <button type="button" class="btn-primary" :disabled="statusSaving" @click="confirmToggleStatus">
            {{ statusSaving ? "处理中…" : "确认" }}
          </button>
        </div>
      </div>
    </div>
    </Transition>
  </section>
</template>
