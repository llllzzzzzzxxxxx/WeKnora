-- Migration 000079 (SQLite) down: drop knowledge_folders and knowledges.folder_id.
DROP INDEX IF EXISTS idx_knowledge_folders_deleted_at;
DROP INDEX IF EXISTS idx_knowledge_folders_tenant_id;
DROP INDEX IF EXISTS idx_knowledge_folders_parent;
DROP INDEX IF EXISTS idx_knowledge_folders_parent_name;
DROP TABLE IF EXISTS knowledge_folders;

DROP INDEX IF EXISTS idx_knowledges_folder_id;
ALTER TABLE knowledges DROP COLUMN folder_id;
