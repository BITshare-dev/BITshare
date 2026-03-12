<script setup lang="ts">
export interface FolderTreeNode {
  id: string;
  name: string;
  status: string;
  tags: string[];
  folders: FolderTreeNode[];
  files: Array<{
    id: string;
    title: string;
    original_name: string;
    status: string;
    size: number;
    download_count: number;
  }>;
}

defineOptions({
  name: "AdminFolderTreeNode",
});

const props = defineProps<{
  node: FolderTreeNode;
  tagInputs: Record<string, string>;
}>();

defineEmits<{
  saveTags: [folderId: string];
}>();

function formatSize(size: number) {
  if (size < 1024) {
    return `${size} B`;
  }
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`;
  }
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}
</script>

<template>
  <article class="rounded-[22px] border border-slate-800 bg-slate-900 p-5">
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <h4 class="text-lg font-semibold text-white">{{ props.node.name }}</h4>
        <p class="mt-1 text-sm text-slate-400">{{ props.node.status }}</p>
      </div>

      <div class="min-w-[280px] flex-1">
        <label class="block">
          <span class="mb-2 block text-xs uppercase tracking-[0.18em] text-slate-500">文件夹 Tag</span>
          <div class="flex gap-2">
            <input
              v-model="props.tagInputs[props.node.id]"
              class="min-w-0 flex-1 rounded-2xl border border-slate-700 bg-slate-950 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
            />
            <button
              class="rounded-2xl bg-blue-500 px-4 py-3 text-sm font-semibold text-slate-950 transition hover:bg-blue-400"
              @click="$emit('saveTags', props.node.id)"
            >
              保存
            </button>
          </div>
        </label>
      </div>
    </div>

    <div class="mt-4 flex flex-wrap gap-2">
      <span
        v-for="tag in props.node.tags"
        :key="tag"
        class="rounded-full bg-blue-950 px-3 py-1 text-xs font-medium text-blue-200"
      >
        {{ tag }}
      </span>
      <span
        v-if="props.node.tags.length === 0"
        class="rounded-full bg-slate-800 px-3 py-1 text-xs font-medium text-slate-400"
      >
        无 Tag
      </span>
    </div>

    <div class="mt-5 grid gap-3 lg:grid-cols-[1fr_1fr]">
      <div class="rounded-2xl bg-slate-950 p-4">
        <p class="text-xs uppercase tracking-[0.18em] text-slate-500">文件</p>
        <ul class="mt-3 space-y-2 text-sm text-slate-300">
          <li v-for="file in props.node.files" :key="file.id">
            {{ file.original_name }} · {{ formatSize(file.size) }} · 下载 {{ file.download_count }}
          </li>
          <li v-if="props.node.files.length === 0" class="text-slate-500">无文件</li>
        </ul>
      </div>

      <div class="rounded-2xl bg-slate-950 p-4">
        <p class="text-xs uppercase tracking-[0.18em] text-slate-500">子目录</p>
        <div class="mt-3 space-y-3">
          <AdminFolderTreeNode
            v-for="child in props.node.folders"
            :key="child.id"
            :node="child"
            :tag-inputs="props.tagInputs"
            @save-tags="$emit('saveTags', $event)"
          />
          <p v-if="props.node.folders.length === 0" class="text-sm text-slate-500">无子目录</p>
        </div>
      </div>
    </div>
  </article>
</template>
