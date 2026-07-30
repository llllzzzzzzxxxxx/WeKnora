-- Migration 000079: multi-level folder support for document knowledge bases.
-- Description:
--   * Add knowledge_folders table: first-class directory nodes that the user
--     can create / rename / move / delete independently of any document.
--     A folder is identified by a uuid and linked to its parent via
--     parent_id ("" = KB root). The materialized /"-joined path is kept for
--     cheap display and sort order, and is recomputed when a subtree is
--     renamed or moved.
--   * Add knowledges.folder_id: nullable FK to knowledge_folders.id. Empty
--     string means "KB root" (the existing default). The column is indexed
--     because folder filtering is the dominant query for the document list
--     and chat-the-folder mention flows.

DO $$ BEGIN RAISE NOTICE '[Migration 000079] Applying knowledge folder schema'; END $$;

ALTER TABLE knowledges ADD COLUMN IF NOT EXISTS folder_id VARCHAR(36) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_knowledges_folder_id
    ON knowledges (knowledge_base_id, folder_id);

-- ---------------------------------------------------------------------------
-- knowledge_folders table.
-- Mirrors wiki_folders so the API surface, repository methods, and rename /
-- move semantics can be cloned wholesale. Tenant + soft-delete scopes are
-- the same as wiki_folders; ordering by (depth, path) keeps the directory
-- tree in human-readable order when the UI renders a flat list.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS knowledge_folders (
    id                VARCHAR(36) PRIMARY KEY,
    tenant_id         BIGINT NOT NULL DEFAULT 0,
    knowledge_base_id VARCHAR(36) NOT NULL,
    parent_id         VARCHAR(36) NOT NULL DEFAULT '',
    name              VARCHAR(255) NOT NULL,
    path              VARCHAR(1024) NOT NULL DEFAULT '',
    depth             INT NOT NULL DEFAULT 0,
    sort_order        INT NOT NULL DEFAULT 0,
    created_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMP WITH TIME ZONE
);

-- A folder name is unique among its live siblings under the same parent.
CREATE UNIQUE INDEX IF NOT EXISTS idx_knowledge_folders_parent_name
    ON knowledge_folders (knowledge_base_id, parent_id, name)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_knowledge_folders_parent
    ON knowledge_folders (knowledge_base_id, parent_id);

CREATE INDEX IF NOT EXISTS idx_knowledge_folders_tenant_id
    ON knowledge_folders (tenant_id);

CREATE INDEX IF NOT EXISTS idx_knowledge_folders_deleted_at
    ON knowledge_folders (deleted_at);

DO $$ BEGIN RAISE NOTICE '[Migration 000079] knowledge folder schema applied successfully'; END $$;
