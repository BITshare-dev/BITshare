import { defineStore } from "pinia";
import { ref } from "vue";

export const useSessionStore = defineStore("session", () => {
  const authenticated = ref(false);
  const displayName = ref("");
  const adminId = ref("");
  const role = ref("");
  const status = ref("");
  const permissions = ref<string[]>([]);

  function setAuthenticated(value: boolean, name = "", payload?: {
    adminId?: string;
    role?: string;
    status?: string;
    permissions?: string[];
  }) {
    authenticated.value = value;
    displayName.value = name;
    adminId.value = value ? (payload?.adminId ?? "") : "";
    role.value = value ? (payload?.role ?? "") : "";
    status.value = value ? (payload?.status ?? "") : "";
    permissions.value = value ? [...(payload?.permissions ?? [])] : [];
  }

  function reset() {
    setAuthenticated(false);
  }

  return {
    authenticated,
    displayName,
    adminId,
    role,
    status,
    permissions,
    setAuthenticated,
    reset,
  };
});
