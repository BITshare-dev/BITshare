<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";

import PageHeader from "../../components/ui/PageHeader.vue";
import SurfaceCard from "../../components/ui/SurfaceCard.vue";
import { httpClient } from "../../lib/http/client";
import { readApiError } from "../../lib/http/helpers";
import { useSessionStore } from "../../stores/session";

interface AdminProfileResponse {
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
const profileSaving = ref(false);
const passwordSaving = ref(false);
const error = ref("");
const success = ref("");

const profileForm = reactive({
  displayName: "",
  avatarUrl: "",
});

const passwordForm = reactive({
  newPassword: "",
  confirmPassword: "",
});

const passwordDirty = computed(() => {
  return (
    passwordForm.newPassword !== "" ||
    passwordForm.confirmPassword !== ""
  );
});

const profileDirty = computed(() => {
  return (
    profileForm.displayName.trim() !== sessionStore.displayName.trim() ||
    profileForm.avatarUrl.trim() !== sessionStore.avatarUrl.trim()
  );
});

const passwordValid = computed(() => {
  return (
    passwordForm.newPassword.length >= 8 &&
    passwordForm.confirmPassword !== "" &&
    passwordForm.newPassword === passwordForm.confirmPassword
  );
});

onMounted(() => {
  resetProfileForm();
});

function resetProfileForm() {
  profileForm.displayName = sessionStore.displayName;
  profileForm.avatarUrl = sessionStore.avatarUrl;
}

async function onAvatarSelected(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) return;
  if (!file.type.startsWith("image/")) {
    error.value = "头像必须是图片文件。";
    return;
  }

  const dataUrl = await readFileAsDataURL(file);
  profileForm.avatarUrl = dataUrl;
}

function clearAvatar() {
  profileForm.avatarUrl = "";
}

async function saveProfile() {
  profileSaving.value = true;
  error.value = "";
  success.value = "";
  try {
    const response = await httpClient.request<AdminProfileResponse>("/admin/account/profile", {
      method: "PATCH",
      body: {
        display_name: profileForm.displayName,
        avatar_url: profileForm.avatarUrl,
      },
    });
    applySessionProfile(response.admin);
    resetProfileForm();
    success.value = "账号资料已更新。";
  } catch (err: unknown) {
    error.value = readApiError(err, "更新账号资料失败。");
  } finally {
    profileSaving.value = false;
  }
}

async function changePassword() {
  if (!passwordValid.value) {
    error.value = passwordForm.newPassword !== passwordForm.confirmPassword ? "两次输入的新密码不一致。" : "请填写完整且有效的新密码。";
    success.value = "";
    return;
  }

  passwordSaving.value = true;
  error.value = "";
  success.value = "";
  try {
    await httpClient.post("/admin/session/change-password", {
      new_password: passwordForm.newPassword,
    });
    passwordForm.newPassword = "";
    passwordForm.confirmPassword = "";
    success.value = "密码已更新。";
  } catch (err: unknown) {
    error.value = readApiError(err, "修改密码失败。");
  } finally {
    passwordSaving.value = false;
  }
}

function applySessionProfile(admin: AdminProfileResponse["admin"]) {
  sessionStore.setAuthenticated(true, admin.display_name || admin.username, {
    username: admin.username,
    adminId: admin.id,
    avatarUrl: admin.avatar_url,
    role: admin.role,
    status: admin.status,
    permissions: admin.permissions,
  });
}

function readFileAsDataURL(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ""));
    reader.onerror = () => reject(new Error("file read failed"));
    reader.readAsDataURL(file);
  });
}
</script>

<template>
  <section class="space-y-8">
    <PageHeader
      eyebrow="Account"
      title="账号设置"
    />

    <section class="grid gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(0,420px)]">
      <SurfaceCard>
        <div>
          <h2 class="text-lg font-semibold text-slate-900">基本资料</h2>
        </div>

        <div class="mt-6 flex items-center gap-4">
          <div class="flex h-24 w-24 items-center justify-center overflow-hidden rounded-full border border-slate-200 bg-slate-100 text-3xl font-semibold text-slate-700">
            <img v-if="profileForm.avatarUrl" :src="profileForm.avatarUrl" alt="头像预览" class="h-full w-full object-cover" />
            <span v-else>{{ sessionStore.displayName.slice(0, 1).toUpperCase() || "A" }}</span>
          </div>
          <div class="flex flex-col gap-3">
            <label class="inline-flex h-10 cursor-pointer items-center rounded-xl border border-slate-200 px-4 text-sm font-medium text-slate-600 transition hover:bg-slate-50 hover:text-slate-900">
              <span>更改头像</span>
              <input type="file" accept="image/*" class="hidden" @change="onAvatarSelected" />
            </label>
            <button
              type="button"
              class="inline-flex h-10 items-center rounded-xl border border-slate-200 px-4 text-sm font-medium text-slate-600 transition hover:bg-slate-50 hover:text-slate-900"
              @click="clearAvatar"
            >
              移除头像
            </button>
          </div>
        </div>

        <div class="mt-6 grid gap-4">
          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">标示ID</label>
            <input :value="sessionStore.username" class="field bg-slate-50" disabled />
          </div>
          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">用户名（对外展示）</label>
            <input v-model="profileForm.displayName" class="field" placeholder="请输入用户名（对外展示）" />
          </div>
        </div>

        <div class="mt-6 flex gap-3">
          <button type="button" class="btn-primary" :disabled="profileSaving || !profileDirty" @click="saveProfile">
            {{ profileSaving ? "更新中…" : "确认更新" }}
          </button>
        </div>
      </SurfaceCard>

      <SurfaceCard>
        <div>
          <h2 class="text-lg font-semibold text-slate-900">修改密码</h2>
          <p class="mt-1 text-sm text-slate-500">新密码至少 8 位。修改后立即对当前账号生效。</p>
        </div>

        <form class="mt-6 space-y-4" @submit.prevent="changePassword">
          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">新密码</label>
            <input v-model="passwordForm.newPassword" type="password" class="field" placeholder="至少 8 位" />
          </div>
          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">确认新密码</label>
            <input v-model="passwordForm.confirmPassword" type="password" class="field" placeholder="再次输入新密码" />
          </div>
          <button type="submit" class="btn-primary" :disabled="passwordSaving || !passwordValid">
            {{ passwordSaving ? "更新中…" : "确认更新" }}
          </button>
        </form>

        <p v-if="passwordDirty && !passwordValid" class="mt-4 text-sm text-slate-500">
          新密码至少 8 位，且两次输入保持一致。
        </p>
      </SurfaceCard>
    </section>

    <p v-if="success" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ success }}</p>
    <p v-if="error" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ error }}</p>
  </section>
</template>
