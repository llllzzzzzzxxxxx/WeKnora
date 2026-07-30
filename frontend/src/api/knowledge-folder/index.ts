import { get, post, put, del } from "../../utils/request";

// Knowledge Folder Types (mirroring backend types in internal/types/knowledge_folder.go)

export interface KnowledgeFolder {
  id: string;
  tenant_id: number;
  knowledge_base_id: string;
  parent_id: string;
  name: string;
  path: string;
  depth: number;
  sort_order: number;
  created_at: string;
  updated_at: string;
}

export interface KnowledgeFolderNode extends KnowledgeFolder {
  document_count: number;
  has_children: boolean;
}

export interface KnowledgeFolderTreeNode extends KnowledgeFolderNode {
  children?: KnowledgeFolderTreeNode[];
}

export interface KnowledgeFolderListResponse {
  parent_id: string;
  folders: KnowledgeFolderNode[];
}

export interface KnowledgeFolderTreeResponse {
  knowledge_base_id: string;
  root: KnowledgeFolderTreeNode[];
}

export interface KnowledgeFolderCreateRequest {
  parent_id?: string;
  name: string;
}

export interface KnowledgeFolderUpdateRequest {
  name?: string;
  parent_id?: string;
  move_parent?: boolean;
}

export interface MoveKnowledgeToFolderRequest {
  knowledge_ids: string[];
  folder_id: string;
}

export interface MoveKnowledgeToFolderResponse {
  acknowledged: string[];
  folder_id: string;
}

/**
 * List direct child folders of a parent folder inside a document KB.
 * Pass empty parentId to list root-level folders.
 */
export function listKnowledgeFolders(kbId: string, parentId = '') {
  const query = new URLSearchParams();
  if (parentId) query.set('parent_id', parentId);
  const qs = query.toString();
  return get<KnowledgeFolderListResponse>(
    `/api/v1/knowledge-bases/${kbId}/folders${qs ? '?' + qs : ''}`
  );
}

/**
 * List the full folder tree for a document KB.
 * Used by the document list sidebar.
 */
export function listKnowledgeFolderTree(kbId: string, parentId = '') {
  const query = new URLSearchParams();
  if (parentId) query.set('parent_id', parentId);
  const qs = query.toString();
  return get<KnowledgeFolderTreeResponse>(
    `/api/v1/knowledge-bases/${kbId}/folders/tree${qs ? '?' + qs : ''}`
  );
}

/**
 * Create a new folder in a document KB.
 */
export function createKnowledgeFolder(kbId: string, data: KnowledgeFolderCreateRequest) {
  return post<{ success: boolean; data: KnowledgeFolder }>(
    `/api/v1/knowledge-bases/${kbId}/folders`,
    data
  );
}

/**
 * Update (rename/move) a folder in a document KB.
 */
export function updateKnowledgeFolder(
  kbId: string,
  folderId: string,
  data: KnowledgeFolderUpdateRequest
) {
  return put<{ success: boolean; data: KnowledgeFolder }>(
    `/api/v1/knowledge-bases/${kbId}/folders/${folderId}`,
    data
  );
}

/**
 * Delete a folder in a document KB.
 * Fails if the folder contains documents or child folders.
 */
export function deleteKnowledgeFolder(kbId: string, folderId: string) {
  return del<{ success: boolean; message: string }>(
    `/api/v1/knowledge-bases/${kbId}/folders/${folderId}`
  );
}

/**
 * Move one or more documents into a folder.
 * Pass empty folderId to move documents to KB root.
 */
export function moveKnowledgeToFolder(
  kbId: string,
  data: MoveKnowledgeToFolderRequest
) {
  return post<MoveKnowledgeToFolderResponse>(
    `/api/v1/knowledge-bases/${kbId}/folders/move-knowledge`,
    data
  );
}
