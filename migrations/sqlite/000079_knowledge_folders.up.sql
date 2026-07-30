-- Migration 000079 (SQLite): multi-level folder support for document knowledge bases.

ALTER TABLE knowledges ADD COLUMN folder_id VARCHAR(36) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_knowledges_folder_id
    ON knowledges (knowledge_base_id, folder_id);

CREATE TABLE IF NOT EXISTS knowledge_folders (
    id                VARCHAR(36) PRIMARY KEY,
    tenant_id         INTEGER NOT NULL DEFAULT 0,
    knowledge_base_id VARCHAR(36) NOT NULL,
    parent_id         VARCHAR(36) NOT NULL DEFAULT '',
    name              VARCHAR(255) NOT NULL,
    path              VARCHAR(1024) NOT NULL DEFAULT '',
    depth             INTEGER NOT NULL DEFAULT 0,
    sort_order        INTEGER NOT NULL DEFAULT 0,
    created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at        DATETIME
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_knowledge_folders_parent_name
    ON knowledge_folders (knowledge_base_id, parent_id, name)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_knowledge_folders_parent
    ON knowledge_folders (knowledge_base_id, parent_id);

CREATE INDEX IF NOT EXISTS idx_knowledge_folders_tenant_id
    ON knowledge_folders (tenant_id);

CREATE INDEX IF NOT EXISTS idx_knowledge_folders_deleted_at
    ON knowledge_folders (deleted_at);
