import { defineStore } from "pinia";
import { ref } from "vue";

export const useSessionStore = defineStore("session", () => {
  const authenticated = ref(false);
  const displayName = ref("");

  function setAuthenticated(value: boolean, name = "") {
    authenticated.value = value;
    displayName.value = name;
  }

  return {
    authenticated,
    displayName,
    setAuthenticated,
  };
});
