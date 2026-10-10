<template>
  <li>
    <div class="node">
      <button
        type="button"
        class="twist"
        :class="{ leaf: !isDir }"
        :aria-expanded="isDir ? String(open) : undefined"
        :aria-label="isDir ? (open ? 'Collapse ' : 'Expand ') + node.name : undefined"
        :tabindex="isDir ? 0 : -1"
        @click="open = !open"
      ></button>
      <input
        :id="id"
        type="checkbox"
        :checked="state === 'all'"
        :indeterminate.prop="state === 'some'"
        @change="$emit('toggle', node, $event.target.checked)"
      />
      <label :for="id">{{ node.name }}{{ isDir ? "/" : "" }}</label>
    </div>
    <ul v-if="isDir && open">
      <FileTreeNode
        v-for="child in children"
        :key="child.path"
        :node="child"
        :chosen="chosen"
        @toggle="(n, on) => $emit('toggle', n, on)"
      />
    </ul>
  </li>
</template>

<script>
let counter = 0;

export default {
  name: "FileTreeNode",
  props: {
    node: { type: Object, required: true },
    chosen: { type: Object, required: true },
  },
  emits: ["toggle"],
  data: () => ({ open: true, id: "ft" + counter++ }),
  computed: {
    isDir() {
      return !this.node.leaf;
    },
    children() {
      return [...this.node.children.values()].sort(
        (a, b) => Number(!a.leaf === false) - Number(!b.leaf === false) || a.name.localeCompare(b.name)
      );
    },
    state() {
      const n = this.node.files.filter((f) => this.chosen.has(f)).length;
      return n === 0 ? "none" : n === this.node.files.length ? "all" : "some";
    },
  },
};
</script>

<style>
.file-tree .node {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 2px 0;
}
.file-tree .node label {
  cursor: pointer;
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}
.file-tree .twist {
  width: 16px;
  height: 16px;
  flex: none;
  background: none;
  border: 0;
  cursor: pointer;
  padding: 0;
  position: relative;
}
.file-tree .twist::after {
  content: "";
  position: absolute;
  left: 4px;
  top: 4px;
  width: 6px;
  height: 6px;
  border-right: 1.5px solid currentColor;
  border-bottom: 1.5px solid currentColor;
  transform: rotate(-45deg);
  opacity: 0.7;
}
.file-tree .twist[aria-expanded="true"]::after {
  transform: rotate(45deg);
}
.file-tree .twist.leaf {
  visibility: hidden;
}
.file-tree .node input {
  accent-color: #04a3e5;
  margin: 0;
}
</style>
