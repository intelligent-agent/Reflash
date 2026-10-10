<template>
  <div class="file-tree">
    <div class="presets">
      <button type="button" class="link" @click="all()">Everything</button>
      <button type="button" class="link" @click="configOnly()">Config only</button>
      <button type="button" class="link" @click="none()">Nothing</button>
    </div>
    <p v-if="!files.length" class="empty">No files.</p>
    <ul v-else class="tree">
      <FileTreeNode
        v-for="node in roots"
        :key="node.path"
        :node="node"
        :chosen="chosen"
        @toggle="toggle"
      />
    </ul>
  </div>
</template>

<script>
import { buildTree, visibleRoots, isConfigFile } from "../fileTree";
import FileTreeNode from "./FileTreeNode.vue";

export default {
  name: "FileTree",
  components: { FileTreeNode },
  props: {
    // Every path that can be chosen.
    files: { type: Array, default: () => [] },
    // The paths that are.
    modelValue: { type: Array, default: () => [] },
  },
  emits: ["update:modelValue"],
  computed: {
    roots() {
      return visibleRoots(buildTree(this.files));
    },
    chosen() {
      return new Set(this.modelValue);
    },
  },
  methods: {
    all() {
      this.$emit("update:modelValue", [...this.files]);
    },
    none() {
      this.$emit("update:modelValue", []);
    },
    configOnly() {
      this.$emit("update:modelValue", this.files.filter(isConfigFile));
    },
    // A folder or a file, on or off: its files, all together.
    toggle(node, on) {
      const next = new Set(this.modelValue);
      node.files.forEach((f) => (on ? next.add(f) : next.delete(f)));
      this.$emit("update:modelValue", this.files.filter((f) => next.has(f)));
    },
  },
};
</script>

<style>
.file-tree {
  border: 1px solid rgba(var(--w-base-color-rgb), 0.2);
  padding: 8px 10px;
  max-height: 260px;
  overflow: auto;
  text-align: left;
  font-size: 0.9em;
}
.file-tree .presets {
  display: flex;
  gap: 14px;
  padding-bottom: 6px;
  margin-bottom: 6px;
  border-bottom: 1px solid rgba(var(--w-base-color-rgb), 0.2);
}
.file-tree .link {
  background: none;
  border: 0;
  padding: 0;
  color: #04a3e5;
  cursor: pointer;
  font: inherit;
}
.file-tree .tree,
.file-tree .tree ul {
  list-style: none;
  margin: 0;
  padding: 0;
}
.file-tree .tree ul {
  padding-left: 22px;
}
.file-tree .empty {
  opacity: 0.7;
  margin: 4px 0;
}
</style>
