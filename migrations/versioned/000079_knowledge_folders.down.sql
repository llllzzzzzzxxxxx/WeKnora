-- Migration 000079 down: drop knowledge_folders and the folder_id column on knowledges.
DROP INDEX IF EXISTS idx_knowledge_folders_deleted_at;
DROP INDEX IF EXISTS idx_knowledge_folders_tenant_id;
DROP INDEX IF EXISTS idx_knowledge_folders_parent;
DROP INDEX IF EXISTS idx_knowledge_folders_parent_name;
DROP TABLE IF EXISTS knowledge_folders;

DROP INDEX IF EXISTS idx_knowledges_folder_id;
ALTER TABLE knowledges DROP COLUMN IF EXISTS folder_id;
