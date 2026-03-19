<script setup lang="ts">
import { computed, onMounted } from "vue";
import { RouterView, useRoute } from "vue-router";

import Navbar from "../components/layout/Navbar.vue";
import { httpClient } from "../lib/http/client";

const route = useRoute();
const viewKey = computed(() => {
  if (route.name === "public-file-detail") {
    return `file:${String(route.params.fileID ?? "")}`;
  }
  return String(route.name ?? route.path);
});

const links = [
  { to: "/", label: "首页" },
  { to: "/upload", label: "回执查询" },
];

onMounted(() => {
  void trackVisit();
});

async function trackVisit() {
  try {
    await httpClient.request("/visits", {
      method: "POST",
      body: {
        scope: "public",
        path: route.path,
      },
    });
  } catch {
    // Ignore analytics failures.
  }
}
</script>

<template>
  <div class="app-shell">
    <Navbar
      :items="links"
      :current-path="route.path"
      github-href="https://github.com/zzzzquan/OpenShare"
    />

    <main class="pt-16">
      <RouterView :key="viewKey" />
    </main>
  </div>
</template>
