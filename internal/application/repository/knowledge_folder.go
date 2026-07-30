// package repository persists the application's domain entities into the
// underlying SQL database.
//
// knowledge_folder.go: persistence layer for the knowledge folder
// hierarchy. The methods mirror the wiki folder repository so the service
// layer can re-use the same rename/move/delete algorithms the wiki already
// trusts in production.
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// knowledgeFolderRepository is the GORM-backed implementation of
// KnowledgeFolderRepository. It mirrors *wikiPageRepository's folder methods
// so the service layer can lift the rename / move / delete algorithms
// without modification.
type knowledgeFolderRepository struct {
	db *gorm.DB
}

// NewKnowledgeFolderRepository constructs a KnowledgeFolderRepository
// backed by the given GORM handle.
func NewKnowledgeFolderRepository(db *gorm.DB) interfaces.KnowledgeFolderRepository {
	return &knowledgeFolderRepository{db: db}
}

// CreateFolder inserts a new folder row.
func (r *knowledgeFolderRepository) CreateFolder(ctx context.Context, folder *types.KnowledgeFolder) error {
	return r.db.WithContext(ctx).Create(folder).Error
}

// GetFolderByID returns a single folder by ID, scoped to the supplied KB
// to make cross-KB collisions impossible.
func (r *knowledgeFolderRepository) GetFolderByID(ctx context.Context, kbID string, id string) (*types.KnowledgeFolder, error) {
	var folder types.KnowledgeFolder
	if err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ? AND id = ?", kbID, id).
		First(&folder).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, interfaces.ErrKnowledgeFolderNotFound
		}
		return nil, err
	}
	return &folder, nil
}

// GetChildFolderByName returns the direct child of parentID with the given
// name. Used by the (kb, parent, name) uniqueness pre-check.
func (r *knowledgeFolderRepository) GetChildFolderByName(
	ctx context.Context, kbID string, parentID string, name string,
) (*types.KnowledgeFolder, error) {
	var folder types.KnowledgeFolder
	if err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ? AND parent_id = ? AND name = ?", kbID, parentID, name).
		First(&folder).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, interfaces.ErrKnowledgeFolderNotFound
		}
		return nil, err
	}
	return &folder, nil
}

// ListChildFolders returns the direct children of parentID (parentID = ""
// = KB root) ordered by (sort_order, name) so the browser's tree is
// deterministic across requests.
func (r *knowledgeFolderRepository) ListChildFolders(
	ctx context.Context, kbID string, parentID string,
) ([]*types.KnowledgeFolder, error) {
	var folders []*types.KnowledgeFolder
	if err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ? AND parent_id = ?", kbID, parentID).
		Order("sort_order ASC").
		Order("name ASC").
		Find(&folders).Error; err != nil {
		return nil, err
	}
	return folders, nil
}

// ListAllFolders returns every folder in the KB, ordered by (depth, path).
// The service uses this on rename / move to recompute the materialized
// path of the entire subtree in a single read.
func (r *knowledgeFolderRepository) ListAllFolders(ctx context.Context, kbID string) ([]*types.KnowledgeFolder, error) {
	var folders []*types.KnowledgeFolder
	if err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ?", kbID).
		Order("depth ASC").
		Order("path ASC").
		Find(&folders).Error; err != nil {
		return nil, err
	}
	return folders, nil
}

// UpdateFolder updates parent_id, name, path, depth, sort_order on the row
// identified by the receiver's ID. Returns interfaces.ErrKnowledgeFolderNotFound when
// no row matches so the service can map to a 404.
func (r *knowledgeFolderRepository) UpdateFolder(ctx context.Context, folder *types.KnowledgeFolder) error {
	result := r.db.WithContext(ctx).
		Model(&types.KnowledgeFolder{}).
		Where("id = ?", folder.ID).
		Updates(map[string]interface{}{
			"parent_id":  folder.ParentID,
			"name":       folder.Name,
			"path":       folder.Path,
			"depth":      folder.Depth,
			"sort_order": folder.SortOrder,
			"updated_at": folder.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return interfaces.ErrKnowledgeFolderNotFound
	}
	return nil
}

// DeleteFolder atomically soft-deletes an empty folder. The emptiness check
// runs in the same SQL statement as the soft delete so a move-in or page
// create can race the service's earlier checks without leaving a dangling
// folder_id. The unique violation on (kb, parent, name) is the contention
// boundary; renaming a sibling while we delete is impossible.
func (r *knowledgeFolderRepository) DeleteFolder(ctx context.Context, kbID string, id string) error {
	result := r.db.WithContext(ctx).Exec(`
UPDATE knowledge_folders
SET deleted_at = ?
WHERE knowledge_base_id = ? AND id = ? AND deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM knowledges
    WHERE knowledge_base_id = ? AND folder_id = ? AND deleted_at IS NULL
  )
  AND NOT EXISTS (
    SELECT 1 FROM knowledge_folders AS child
    WHERE child.knowledge_base_id = ? AND child.parent_id = ? AND child.deleted_at IS NULL
  )`, time.Now(), kbID, id, kbID, id, kbID, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var count int64
		if err := r.db.WithContext(ctx).Model(&types.KnowledgeFolder{}).
			Where("knowledge_base_id = ? AND id = ?", kbID, id).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return interfaces.ErrKnowledgeFolderNotFound
		}
		return interfaces.ErrKnowledgeFolderNotEmpty
	}
	return nil
}

// ListDescendantFolderIDs walks the parent chain in Go from each of the
// supplied folder IDs and returns the union of input + descendants. Doing
// the walk in Go (rather than a recursive CTE) keeps the query portable
// across PostgreSQL and SQLite without a separate migration: both backends
// already support the (kb, parent_id) index this function relies on.
func (r *knowledgeFolderRepository) ListDescendantFolderIDs(
	ctx context.Context, kbID string, folderIDs []string,
) ([]string, error) {
	if len(folderIDs) == 0 {
		return nil, nil
	}
	all, err := r.ListAllFolders(ctx, kbID)
	if err != nil {
		return nil, err
	}
	byParent := make(map[string][]*types.KnowledgeFolder, len(all))
	for _, f := range all {
		byParent[f.ParentID] = append(byParent[f.ParentID], f)
	}
	seen := make(map[string]bool, len(folderIDs))
	queue := append([]string(nil), folderIDs...)
	out := make([]string, 0, len(folderIDs))
	for _, id := range queue {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
		for _, child := range byParent[id] {
			if !seen[child.ID] {
				seen[child.ID] = true
				out = append(out, child.ID)
				queue = append(queue, child.ID)
			}
		}
	}
	return out, nil
}

