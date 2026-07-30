<template>
  <div class="doc-folder-tree-item">
    <!-- Folder row -->
    <div
      :class="['folder-row', { selected: isSelected, 'has-children': node.has_children }]"
      :style="{ paddingLeft: `${(node.depth || 0) * 16 + 8}px` }"
      @click="handleSelect"
      @contextmenu.prevent="showContextMenu"
    >
      <!-- Expand/collapse toggle -->
      <span
        v-if="node.has_children || hasChildrenComputed"
        :class="['expand-toggle', { expanded: isExpanded }]"
        @click.stop="toggleExpand"
      >
        <t-icon name="chevron-right" />
      </span>
      <span v-else class="expand-spacer"></span>

      <!-- Folder icon -->
      <t-icon :name="isExpanded ? 'folder-open' : 'folder'" class="folder-icon" />

      <!-- Folder name -->
      <span class="folder-name" :title="node.name">{{ node.name }}</span>

      <!-- Document count badge -->
      <t-badge
        v-if="node.document_count > 0"
        :count="node.document_count"
        :max-count="99"
        class="doc-count-badge"
      />

      <!-- Actions button (visible on hover) -->
      <div v-if="canEdit" class="folder-actions">
        <t-popup
          trigger="click"
          placement="bottom-start"
          :overlay-class-name="'doc-folder-action-popup'"
        >
          <t-button variant="text" size="small" class="action-btn">
            <t-icon name="more" />
          </t-button>
          <template #content>
            <div class="folder-action-menu">
              <div class="action-item" @click="handleCreateChild">
                <t-icon name="folder-add" />
                <span>{{ t('knowledgeBase.newSubfolder') || '新建子文件夹' }}</span>
              </div>
              <div class="action-item" @click="handleRename">
                <t-icon name="edit" />
                <span>{{ t('knowledgeBase.renameFolder') || '重命名' }}</span>
              </div>
              <div class="action-item danger" @click="handleDelete">
                <t-icon name="delete" />
                <span>{{ t('knowledgeBase.deleteFolder') || '删除' }}</span>
              </div>
            </div>
          </template>
        </t-popup>
      </div>
    </div>

    <!-- Children (recursive) -->
    <div v-if="isExpanded && node.children && node.children.length > 0" class="folder-children">
      <DocFolderTreeItem
        v-for="child in node.children"
        :key="child.id"
        :node="child"
        :selected-id="selectedId"
        :can-edit="canEdit"
        @select="(id) => emit('select', id)"
        @create-child="(id) => emit('create-child', id)"
        @rename="(id) => emit('rename', id)"
        @delete="(id) => emit('delete', id)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { KnowledgeFolderTreeNode } from '@/api/knowledge-folder';

const props = withDefaults(defineProps<{
  node: KnowledgeFolderTreeNode;
  selectedId?: string | null;
  canEdit?: boolean;
}>(), {
  canEdit: false,
});

const emit = defineEmits<{
  (e: 'select', folderId: string | null): void;
  (e: 'create-child', folderId: string): void;
  (e: 'rename', folderId: string): void;
  (e: 'delete', folderId: string): void;
}>();

const { t } = useI18n();

const isExpanded = ref(false);

const isSelected = computed(() => props.selectedId === props.node.id);
const hasChildrenComputed = computed(() =>
  props.node.children && props.node.children.length > 0
);

const handleSelect = () => {
  emit('select', props.node.id);
};

const toggleExpand = () => {
  isExpanded.value = !isExpanded.value;
};

const showContextMenu = () => {
  // Context menu handled by t-popup on action button
};

const handleCreateChild = () => {
  emit('create-child', props.node.id);
};

const handleRename = () => {
  emit('rename', props.node.id);
};

const handleDelete = () => {
  emit('delete', props.node.id);
};
</script>

<style lang="less" scoped>
.doc-folder-tree-item {
  user-select: none;
}

.folder-row {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 8px;
  border-radius: 4px;
  cursor: pointer;
  transition: background-color 0.15s;

  &:hover {
    background-color: var(--td-bg-color-container-hover);

    .folder-actions {
      opacity: 1;
    }
  }

  &.selected {
    background-color: var(--td-bg-color-container-active);
    color: var(--td-brand-color);
  }
}

.expand-toggle {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  flex-shrink: 0;
  color: var(--td-text-color-placeholder);
  transition: transform 0.15s;

  &.expanded {
    transform: rotate(90deg);
  }

  &:hover {
    color: var(--td-text-color-primary);
  }
}

.expand-spacer {
  width: 16px;
  flex-shrink: 0;
}

.folder-icon {
  flex-shrink: 0;
  color: var(--td-warning-color);
}

.folder-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
}

.doc-count-badge {
  flex-shrink: 0;
  margin-left: 4px;
}

.folder-actions {
  flex-shrink: 0;
  opacity: 0;
  transition: opacity 0.15s;

  .action-btn {
    padding: 2px;
    min-width: auto;
    color: var(--td-text-color-secondary);

    &:hover {
      color: var(--td-text-color-primary);
    }
  }
}

.folder-children {
  margin-left: 0;
}

.folder-action-menu {
  padding: 4px;
  min-width: 140px;

  .action-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 12px;
    border-radius: 4px;
    cursor: pointer;
    font-size: 13px;
    transition: background-color 0.15s;

    &:hover {
      background-color: var(--td-bg-color-container-hover);
    }

    &.danger {
      color: var(--td-error-color);

      &:hover {
        background-color: var(--td-error-color-1);
      }
    }
  }
}
</style>
