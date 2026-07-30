<template>
  <div class="doc-folder-browser">
    <!-- Header with search and new folder button -->
    <div class="doc-folder-header">
      <t-input
        v-model="searchQuery"
        :placeholder="t('knowledgeBase.folderSearchPlaceholder') || '搜索文件夹...'"
        clearable
        @enter="handleSearch"
        @clear="searchQuery = ''"
      >
        <template #prefixIcon>
          <t-icon name="search" />
        </template>
      </t-input>
      <t-button
        v-if="canEdit"
        theme="primary"
        variant="outline"
        size="small"
        @click="showCreateDialog = true"
      >
        <template #icon>
          <t-icon name="folder-add" />
        </template>
        {{ t('knowledgeBase.newFolder') || '新建文件夹' }}
      </t-button>
    </div>

    <!-- Folder tree view -->
    <div class="doc-folder-tree" v-if="!loading && folderTree.length > 0">
      <div
        v-for="node in folderTree"
        :key="node.id"
        class="folder-tree-node"
      >
        <DocFolderTreeItem
          :node="node"
          :selected-id="selectedFolderId"
          :can-edit="canEdit"
          @select="handleSelectFolder"
          @create-child="handleCreateChildFolder"
          @rename="handleRenameFolder"
          @delete="handleDeleteFolder"
        />
      </div>
    </div>

    <!-- Empty state -->
    <div v-else-if="!loading" class="doc-folder-empty">
      <t-icon name="folder-open" size="32px" />
      <p>{{ t('knowledgeBase.noFolders') || '暂无文件夹' }}</p>
      <t-button
        v-if="canEdit"
        theme="primary"
        variant="outline"
        size="small"
        @click="showCreateDialog = true"
      >
        {{ t('knowledgeBase.createFirstFolder') || '创建第一个文件夹' }}
      </t-button>
    </div>

    <!-- Loading state -->
    <div v-if="loading" class="doc-folder-loading">
      <t-loading size="small" />
    </div>

    <!-- Create folder dialog -->
    <t-dialog
      v-model:visible="showCreateDialog"
      :header="t('knowledgeBase.createFolder') || '创建文件夹'"
      :confirm-btn="{ content: t('common.confirm'), theme: 'primary' }"
      :cancel-btn="{ content: t('common.cancel') }"
      @confirm="confirmCreateFolder"
      @close="createDialogName = ''"
    >
      <t-form>
        <t-form-item :label="t('knowledgeBase.folderName') || '文件夹名称'">
          <t-input
            v-model="createDialogName"
            :placeholder="t('knowledgeBase.folderNamePlaceholder') || '请输入文件夹名称'"
            @enter="confirmCreateFolder"
          />
        </t-form-item>
        <t-form-item
          v-if="createParentId !== ''"
          :label="t('knowledgeBase.parentFolder') || '上级文件夹'"
        >
          <span class="parent-folder-label">{{ getFolderName(createParentId) || t('knowledgeBase.rootLevel') || '根目录' }}</span>
        </t-form-item>
      </t-form>
    </t-dialog>

    <!-- Rename folder dialog -->
    <t-dialog
      v-model:visible="showRenameDialog"
      :header="t('knowledgeBase.renameFolder') || '重命名文件夹'"
      :confirm-btn="{ content: t('common.confirm'), theme: 'primary' }"
      :cancel-btn="{ content: t('common.cancel') }"
      @confirm="confirmRenameFolder"
      @close="renameDialogName = ''"
    >
      <t-form>
        <t-form-item :label="t('knowledgeBase.newFolderName') || '新名称'">
          <t-input
            v-model="renameDialogName"
            :placeholder="t('knowledgeBase.folderNamePlaceholder') || '请输入文件夹名称'"
            @enter="confirmRenameFolder"
          />
        </t-form-item>
      </t-form>
    </t-dialog>

    <!-- Delete folder dialog -->
    <t-dialog
      v-model:visible="showDeleteDialog"
      :header="t('knowledgeBase.deleteFolder') || '删除文件夹'"
      :confirm-btn="{ content: t('common.confirm'), theme: 'danger' }"
      :cancel-btn="{ content: t('common.cancel') }"
      @confirm="confirmDeleteFolder"
    >
      <div class="delete-dialog-content">
        <t-icon name="error-circle" style="color: var(--td-warning-color); margin-right: 8px;" />
        <span>
          {{ t('knowledgeBase.deleteFolderConfirm') || '确定要删除文件夹' }}"{{ getFolderName(deleteTargetId) }}"{{ t('knowledgeBase.deleteFolderSuffix') || '吗？文件夹必须为空才能删除。' }}
        </span>
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { MessagePlugin } from 'tdesign-vue-next';
import DocFolderTreeItem from './DocFolderTreeItem.vue';
import {
  listKnowledgeFolderTree,
  createKnowledgeFolder,
  updateKnowledgeFolder,
  deleteKnowledgeFolder,
  type KnowledgeFolderTreeNode,
} from '@/api/knowledge-folder';

const props = defineProps<{
  kbId: string;
  canEdit?: boolean;
  selectedId?: string;
}>();

const emit = defineEmits<{
  (e: 'select', folderId: string | null): void;
}>();

const { t } = useI18n();

const loading = ref(false);
const folderTree = ref<KnowledgeFolderTreeNode[]>([]);
const selectedFolderId = ref<string | null>(props.selectedId || null);
const searchQuery = ref('');

// Dialog states
const showCreateDialog = ref(false);
const createDialogName = ref('');
const createParentId = ref('');

const showRenameDialog = ref(false);
const renameDialogName = ref('');
const renameTargetId = ref('');

const showDeleteDialog = ref(false);
const deleteTargetId = ref('');

// Flatten the tree for search
const flatFolders = computed(() => {
  const result: KnowledgeFolderTreeNode[] = [];
  const traverse = (nodes: KnowledgeFolderTreeNode[]) => {
    for (const node of nodes) {
      result.push(node);
      if (node.children) traverse(node.children);
    }
  };
  traverse(folderTree.value);
  return result;
});

// Get folder name by ID
const getFolderName = (folderId: string): string => {
  if (!folderId) return '';
  const folder = flatFolders.value.find(f => f.id === folderId);
  return folder?.name || '';
};

// Load folder tree
const loadFolders = async () => {
  if (!props.kbId) return;
  loading.value = true;
  try {
    const res = await listKnowledgeFolderTree(props.kbId);
    const body = res as any;
    folderTree.value = body?.root || [];
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('knowledgeBase.loadFoldersError') || '加载文件夹失败');
  } finally {
    loading.value = false;
  }
};

// Handle folder selection
const handleSelectFolder = (folderId: string | null) => {
  selectedFolderId.value = folderId;
  emit('select', folderId);
};

// Handle search
const handleSearch = () => {
  // TODO: Implement folder search
};

// Handle create child folder
const handleCreateChildFolder = (parentId: string) => {
  createParentId.value = parentId;
  createDialogName.value = '';
  showCreateDialog.value = true;
};

// Confirm create folder
const confirmCreateFolder = async () => {
  const name = createDialogName.value.trim();
  if (!name) {
    MessagePlugin.warning(t('knowledgeBase.folderNameRequired') || '请输入文件夹名称');
    return;
  }
  try {
    await createKnowledgeFolder(props.kbId, {
      parent_id: createParentId.value || undefined,
      name,
    });
    MessagePlugin.success(t('knowledgeBase.createFolderSuccess') || '文件夹创建成功');
    showCreateDialog.value = false;
    createDialogName.value = '';
    await loadFolders();
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('knowledgeBase.createFolderError') || '创建文件夹失败');
  }
};

// Handle rename folder
const handleRenameFolder = (folderId: string) => {
  renameTargetId.value = folderId;
  renameDialogName.value = getFolderName(folderId);
  showRenameDialog.value = true;
};

// Confirm rename folder
const confirmRenameFolder = async () => {
  const name = renameDialogName.value.trim();
  if (!name) {
    MessagePlugin.warning(t('knowledgeBase.folderNameRequired') || '请输入文件夹名称');
    return;
  }
  try {
    await updateKnowledgeFolder(props.kbId, renameTargetId.value, { name });
    MessagePlugin.success(t('knowledgeBase.renameFolderSuccess') || '文件夹重命名成功');
    showRenameDialog.value = false;
    renameDialogName.value = '';
    await loadFolders();
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('knowledgeBase.renameFolderError') || '重命名文件夹失败');
  }
};

// Handle delete folder
const handleDeleteFolder = (folderId: string) => {
  deleteTargetId.value = folderId;
  showDeleteDialog.value = true;
};

// Confirm delete folder
const confirmDeleteFolder = async () => {
  try {
    await deleteKnowledgeFolder(props.kbId, deleteTargetId.value);
    MessagePlugin.success(t('knowledgeBase.deleteFolderSuccess') || '文件夹删除成功');
    showDeleteDialog.value = false;
    if (selectedFolderId.value === deleteTargetId.value) {
      handleSelectFolder(null);
    }
    await loadFolders();
  } catch (err: any) {
    MessagePlugin.error(err?.message || t('knowledgeBase.deleteFolderError') || '删除文件夹失败');
  }
};

// Watch for KB changes
watch(() => props.kbId, () => {
  if (props.kbId) {
    loadFolders();
  }
}, { immediate: true });

// Watch for external selectedId changes
watch(() => props.selectedId, (newId) => {
  selectedFolderId.value = newId || null;
});

onMounted(() => {
  if (props.kbId) {
    loadFolders();
  }
});

// Expose for parent component
defineExpose({
  loadFolders,
  selectedFolderId,
});
</script>

<style lang="less" scoped>
.doc-folder-browser {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 12px;
}

.doc-folder-header {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;

  .t-input {
    flex: 1;
  }
}

.doc-folder-tree {
  flex: 1;
  overflow-y: auto;
}

.doc-folder-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: var(--td-text-color-placeholder);
  gap: 12px;

  p {
    margin: 0;
  }
}

.doc-folder-loading {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}

.delete-dialog-content {
  display: flex;
  align-items: flex-start;
}

.parent-folder-label {
  color: var(--td-text-color-secondary);
}
</style>
