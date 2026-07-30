// Package service contains the application's domain service implementations.
//
// knowledge_folder_service.go: encapsulates the operations that the document
// management UI (and the chat mention resolver) need to manipulate the
// directory tree of a document knowledge base. The interface defined in
// internal/types/interfaces/knowledge_folder_service.go is the seam that
// keeps the handler / resolver decoupled from the SQL backing store.
package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// knowledgeFolderService is the default implementation of
// KnowledgeFolderService. It composes a KnowledgeFolderRepository for
// persistence and a KnowledgeRepository for the live document counts that
// decorate every node in the directory tree.
type knowledgeFolderService struct {
	folderRepo  interfaces.KnowledgeFolderRepository
	knowledgeR  interfaces.KnowledgeRepository
}

// NewKnowledgeFolderService constructs a service that can list, create,
// rename, move and delete knowledge folders, plus resolve folder ID picks
// into the document IDs they imply (including descendants when requested).
func NewKnowledgeFolderService(
	folderRepo interfaces.KnowledgeFolderRepository,
	knowledgeRepo interfaces.KnowledgeRepository,
) interfaces.KnowledgeFolderService {
	return &knowledgeFolderService{
		folderRepo: folderRepo,
		knowledgeR: knowledgeRepo,
	}
}

// ListChildFolders returns the direct children of parentID (parentID == "" to
// fetch the KB root level) annotated with the live document count and a
// has-children flag. The two annotations are derived from a single repository
// read so the UI can render the row without a second round-trip.
func (s *knowledgeFolderService) ListChildFolders(
	ctx context.Context,
	kbID string,
	parentID string,
	pageTypes []string,
) ([]types.KnowledgeFolderNode, error) {
	if kbID == "" {
		return nil, errors.New("knowledge base id is required")
	}

	folders, err := s.folderRepo.ListChildFolders(ctx, kbID, parentID)
	if err != nil {
		return nil, fmt.Errorf("list child folders: %w", err)
	}
	if len(folders) == 0 {
		return []types.KnowledgeFolderNode{}, nil
	}

	folderIDs := make([]string, 0, len(folders))
	for _, f := range folders {
		folderIDs = append(folderIDs, f.ID)
	}

	// Pull live counts for each folder and (when includeSubtree is implied by
	// the UI) the descendant sub-tree sums too. We treat the sidebar listing
	// as a "direct children + own documents" view because expanding the tree
	// is a separate call the client triggers when a folder is expanded.
	counts, err := s.knowledgeR.CountDocumentsByFolder(ctx, tenantIDFromContext(ctx), kbID, folderIDs)
	if err != nil {
		return nil, fmt.Errorf("count documents: %w", err)
	}

	// Compute has-children by re-using a single ListAllFolders scan; the
	// repository is already optimized for the (kb, parent_id) index so this
	// is O(kb folders) which is small for any realistic KB.
	all, err := s.folderRepo.ListAllFolders(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("list all folders: %w", err)
	}
	childIndex := make(map[string]bool, len(all))
	for _, f := range all {
		childIndex[f.ParentID] = true
	}

	nodes := make([]types.KnowledgeFolderNode, 0, len(folders))
	for _, f := range folders {
		nodes = append(nodes, types.KnowledgeFolderNode{
			KnowledgeFolder: *f,
			DocumentCount:   counts[f.ID],
			HasChildren:     childIndex[f.ID],
		})
	}
	return nodes, nil
}

// ListFolderTree returns the full subtree rooted at parentID (parentID empty
// to mean KB root). The recursive shape mirrors the wiki folder tree so the
// sidebar component can be shared later if we need to.
func (s *knowledgeFolderService) ListFolderTree(
	ctx context.Context,
	kbID string,
	parentID string,
) ([]*types.KnowledgeFolderTreeNode, error) {
	if kbID == "" {
		return nil, errors.New("knowledge base id is required")
	}

	// Fetch the entire folder set once; the recursive shape is built in Go
	// to keep the SQL portable across PostgreSQL and SQLite without recursive
	// CTEs.
	all, err := s.folderRepo.ListAllFolders(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("list all folders: %w", err)
	}
	if len(all) == 0 {
		return []*types.KnowledgeFolderTreeNode{}, nil
	}

	ids := make([]string, 0, len(all))
	for _, f := range all {
		ids = append(ids, f.ID)
	}
	counts, err := s.knowledgeR.CountDocumentsByFolder(ctx, tenantIDFromContext(ctx), kbID, ids)
	if err != nil {
		return nil, fmt.Errorf("count documents: %w", err)
	}

	// Group by parent_id and sort every bucket by (sort_order, name).
	byParent := make(map[string][]*types.KnowledgeFolder, len(all))
	for _, f := range all {
		byParent[f.ParentID] = append(byParent[f.ParentID], f)
	}
	for _, group := range byParent {
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].SortOrder != group[j].SortOrder {
				return group[i].SortOrder < group[j].SortOrder
			}
			return group[i].Name < group[j].Name
		})
	}

	var build func(parent string) []*types.KnowledgeFolderTreeNode
	build = func(parent string) []*types.KnowledgeFolderTreeNode {
		children := byParent[parent]
		out := make([]*types.KnowledgeFolderTreeNode, 0, len(children))
		for _, f := range children {
			grandchildren := build(f.ID)
			node := &types.KnowledgeFolderTreeNode{
				KnowledgeFolderNode: types.KnowledgeFolderNode{
					KnowledgeFolder: *f,
					DocumentCount:   counts[f.ID],
					HasChildren:     len(grandchildren) > 0,
				},
				Children: nil,
			}
			if len(grandchildren) > 0 {
				node.Children = grandchildren
			}
			out = append(out, node)
		}
		return out
	}
	return build(parentID), nil
}

// CreateFolder validates the (kb, parent, name) uniqueness invariant and
// inserts a new folder row. Name validation enforces a stable shape so the UI
// can render the row without further inspection.
func (s *knowledgeFolderService) CreateFolder(
	ctx context.Context,
	kbID string,
	tenantID uint64,
	parentID string,
	name string,
) (*types.KnowledgeFolder, error) {
	if kbID == "" {
		return nil, errors.New("knowledge base id is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("folder name is required")
	}
	if err := validateKnowledgeFolderName(name); err != nil {
		return nil, err
	}

	var depth int
	var path string
	if parentID == "" {
		path = "/" + name
		depth = 0
	} else {
		parent, err := s.folderRepo.GetFolderByID(ctx, kbID, parentID)
		if err != nil {
			if errors.Is(err, interfaces.ErrKnowledgeFolderNotFound) {
				return nil, fmt.Errorf("parent folder not found: %w", err)
			}
			return nil, fmt.Errorf("get parent folder: %w", err)
		}
		depth = parent.Depth + 1
		path = joinPath(parent.Path, name)
	}

	// Pre-check uniqueness to keep the error message stable across both
	// PostgreSQL (which has a partial unique index) and SQLite.
	if existing, err := s.folderRepo.GetChildFolderByName(ctx, kbID, parentID, name); err == nil && existing != nil {
		return nil, interfaces.ErrKnowledgeFolderConflict
	} else if err != nil && !errors.Is(err, interfaces.ErrKnowledgeFolderNotFound) {
		return nil, fmt.Errorf("check uniqueness: %w", err)
	}

	now := time.Now().UTC()
	folder := &types.KnowledgeFolder{
		ID:              uuid.NewString(),
		TenantID:        tenantID,
		KnowledgeBaseID: kbID,
		ParentID:        parentID,
		Name:            name,
		Path:            path,
		Depth:           depth,
		SortOrder:       0,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.folderRepo.CreateFolder(ctx, folder); err != nil {
		// Race condition: another writer created the same sibling name
		// between the pre-check and the insert. Surface a conflict.
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			return nil, interfaces.ErrKnowledgeFolderConflict
		}
		return nil, fmt.Errorf("create folder: %w", err)
	}
	logger.Infof(ctx, "Created knowledge folder kb=%s parent=%s id=%s name=%s", kbID, parentID, folder.ID, name)
	return folder, nil
}

// RenameOrMoveFolder performs a rename and/or reparent. It rejects cycles and
// sibling name collisions before any write, then asks the repository to
// rewrite the materialized path / depth of the entire subtree in a single
// SQL statement.
func (s *knowledgeFolderService) RenameOrMoveFolder(
	ctx context.Context,
	kbID string,
	id string,
	newName string,
	newParentID string,
	moveParent bool,
) (*types.KnowledgeFolder, error) {
	if kbID == "" || id == "" {
		return nil, errors.New("knowledge base id and folder id are required")
	}

	target, err := s.folderRepo.GetFolderByID(ctx, kbID, id)
	if err != nil {
		return nil, fmt.Errorf("get folder: %w", err)
	}

	resolvedNewName := target.Name
	if name := strings.TrimSpace(newName); name != "" {
		if err := validateKnowledgeFolderName(name); err != nil {
			return nil, err
		}
		resolvedNewName = name
	}

	resolvedNewParent := target.ParentID
	if moveParent {
		resolvedNewParent = newParentID
	}

	// Validate the parent (if moving) and reject cycles.
	if resolvedNewParent == id {
		return nil, errors.New("cannot move folder into itself")
	}
	if resolvedNewParent != "" && resolvedNewParent != target.ParentID {
		newParent, err := s.folderRepo.GetFolderByID(ctx, kbID, resolvedNewParent)
		if err != nil {
			if errors.Is(err, interfaces.ErrKnowledgeFolderNotFound) {
				return nil, errors.New("destination parent not found")
			}
			return nil, fmt.Errorf("get new parent: %w", err)
		}
		// Walk up the parent chain from the destination and ensure we never
		// land on the folder we are moving (or one of its descendants).
		if cycleCollides(ctx, s.folderRepo, kbID, newParent.Path, target.Path) {
			return nil, errors.New("cannot move folder into its descendant")
		}
		_ = newParent
	}

	// Check for sibling name collision under the resolved parent.
	if resolvedNewName != target.Name || resolvedNewParent != target.ParentID {
		if existing, err := s.folderRepo.GetChildFolderByName(ctx, kbID, resolvedNewParent, resolvedNewName); err == nil && existing != nil && existing.ID != id {
			return nil, interfaces.ErrKnowledgeFolderConflict
		} else if err != nil && !errors.Is(err, interfaces.ErrKnowledgeFolderNotFound) {
			return nil, fmt.Errorf("check uniqueness: %w", err)
		}
	}

	// Compute the new (path, depth) of the target folder.
	parentPath := "/"
	parentDepth := -1
	if resolvedNewParent != "" {
		parent, err := s.folderRepo.GetFolderByID(ctx, kbID, resolvedNewParent)
		if err != nil {
			return nil, fmt.Errorf("get parent: %w", err)
		}
		parentPath = parent.Path
		parentDepth = parent.Depth
	}
	oldPath := target.Path
	target.Name = resolvedNewName
	target.ParentID = resolvedNewParent
	target.Path = joinPath(parentPath, resolvedNewName)
	target.Depth = parentDepth + 1
	target.UpdatedAt = time.Now().UTC()

	if err := s.folderRepo.UpdateFolder(ctx, target); err != nil {
		if errors.Is(err, interfaces.ErrKnowledgeFolderNotFound) {
			return nil, err
		}
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			return nil, interfaces.ErrKnowledgeFolderConflict
		}
		return nil, fmt.Errorf("update folder: %w", err)
	}

	// Rewrite the (path, depth) of every descendant so the materialized
	// paths in the subtree still reflect the new parent.
	if err := s.rewriteSubtreePaths(ctx, kbID, oldPath, target.Path); err != nil {
		return nil, fmt.Errorf("rewrite subtree: %w", err)
	}

	updated, err := s.folderRepo.GetFolderByID(ctx, kbID, id)
	if err != nil {
		return nil, fmt.Errorf("reload folder: %w", err)
	}
	return updated, nil
}

// DeleteFolder soft-deletes a folder. The repository enforces emptiness
// (no live documents and no child folders) atomically; we surface the
// underlying error untouched so the handler can map it to a 409.
func (s *knowledgeFolderService) DeleteFolder(ctx context.Context, kbID string, id string) error {
	if kbID == "" || id == "" {
		return errors.New("knowledge base id and folder id are required")
	}
	if err := s.folderRepo.DeleteFolder(ctx, kbID, id); err != nil {
		return err
	}
	return nil
}

// ResolveFolderIDs returns the supplied folder IDs plus every descendant.
// Used by the chat @folder mention to expand a folder pick to its full
// subtree in one go.
func (s *knowledgeFolderService) ResolveFolderIDs(
	ctx context.Context,
	kbID string,
	folderIDs []string,
	includeDescendants bool,
) ([]string, error) {
	if !includeDescendants {
		out := make([]string, 0, len(folderIDs))
		seen := make(map[string]bool, len(folderIDs))
		for _, id := range folderIDs {
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
		return out, nil
	}
	return s.folderRepo.ListDescendantFolderIDs(ctx, kbID, folderIDs)
}

// ResolveKnowledgeIDs expands a folder pick into the deduplicated list of
// knowledge IDs that fall inside those folders (and, when requested, inside
// their subtrees). This is the convenience companion of the same-name method
// on KnowledgeService so chat handlers don't have to fan out into two
// services.
func (s *knowledgeFolderService) ResolveKnowledgeIDs(
	ctx context.Context,
	tenantID uint64,
	kbID string,
	folderIDs []string,
	includeDescendants bool,
) ([]string, error) {
	if len(folderIDs) == 0 {
		return nil, nil
	}
	resolvedFolders, err := s.ResolveFolderIDs(ctx, kbID, folderIDs, includeDescendants)
	if err != nil {
		return nil, fmt.Errorf("resolve folder ids: %w", err)
	}
	ids, err := s.knowledgeR.ListKnowledgeIDsByFolderIDs(ctx, tenantID, kbID, resolvedFolders)
	if err != nil {
		return nil, fmt.Errorf("list knowledge ids by folders: %w", err)
	}
	return ids, nil
}

// rewriteSubtreePaths re-derives Path/Depth for every folder whose path is
// prefixed by oldPrefix. The replacement replaces oldPrefix with newPrefix
// in the string and adjusts depth by (newDepth - oldDepth).
func (s *knowledgeFolderService) rewriteSubtreePaths(
	ctx context.Context, kbID string, oldPrefix string, newPrefix string,
) error {
	if oldPrefix == newPrefix {
		return nil
	}
	all, err := s.folderRepo.ListAllFolders(ctx, kbID)
	if err != nil {
		return err
	}
	for _, f := range all {
		updated := false
		newPath := f.Path
		newDepth := f.Depth
		if f.Path == oldPrefix || strings.HasPrefix(f.Path, oldPrefix+"/") {
			// Replace the prefix segment with the new one.
			if f.Path == oldPrefix {
				newPath = newPrefix
				newDepth = depthOf(newPrefix)
			} else {
				newPath = newPrefix + strings.TrimPrefix(f.Path, oldPrefix)
				newDepth = depthOf(newPrefix) + (f.Depth - depthOf(oldPrefix))
			}
			updated = true
		}
		if updated {
			f.Path = newPath
			f.Depth = newDepth
			f.UpdatedAt = time.Now().UTC()
			if err := s.folderRepo.UpdateFolder(ctx, f); err != nil {
				if errors.Is(err, interfaces.ErrKnowledgeFolderNotFound) {
					continue
				}
				return err
			}
		}
	}
	return nil
}

// cycleCollides returns true when `parentPath` is either equal to or
// nested under `targetPath` (the path of the folder we are moving). Used
// to reject moves that would create a cycle.
func cycleCollides(
	ctx context.Context,
	repo interfaces.KnowledgeFolderRepository,
	kbID string,
	parentPath string,
	targetPath string,
) bool {
	if parentPath == targetPath {
		return true
	}
	if strings.HasPrefix(parentPath, targetPath+"/") {
		return true
	}
	return false
}

// joinPath builds the materialized path by concatenating a parent path and a
// child name. The root has path "/" and the depths are 0, 1, ... matching.
func joinPath(parentPath, name string) string {
	parentPath = strings.TrimRight(parentPath, "/")
	if parentPath == "" {
		return "/" + name
	}
	return parentPath + "/" + name
}

// depthOf derives the depth (number of "/" segments - 1) from a materialized
// path. The root has depth 0, its children have depth 1, and so on.
func depthOf(path string) int {
	if path == "" || path == "/" {
		return 0
	}
	return strings.Count(path, "/")
}

// validateKnowledgeFolderName enforces a stable shape so the UI can render the row
// without further inspection. Empty / over-long names and names containing
// path separators are rejected.
func validateKnowledgeFolderName(name string) error {
	if strings.ContainsAny(name, "/\\") {
		return errors.New("folder name cannot contain '/' or '\\\\'")
	}
	if len(name) > 255 {
		return errors.New("folder name must be at most 255 characters")
	}
	return nil
}

// tenantIDFromContext extracts the tenant id from the request context.
// When the context is missing the tenant key (e.g. tests that pass
// context.Background) the value defaults to 0 — the repository will treat
// the request as tenant-0, which is exactly what the existing services do.
func tenantIDFromContext(ctx context.Context) uint64 {
	if v := ctx.Value(types.TenantIDContextKey); v != nil {
		if id, ok := v.(uint64); ok {
			return id
		}
	}
	return 0
}
