<script setup lang="ts">
import { onMounted } from "vue";
import { RouterView, useRoute } from "vue-router";

import Navbar from "../components/Navbar.vue";
import { httpClient } from "../lib/http/client";

const route = useRoute();

const links = [
  { to: "/", label: "首页" },
  { to: "/upload", label: "上传" },
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
      <RouterView />
    </main>
  </div>
</template>
