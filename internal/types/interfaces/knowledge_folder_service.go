package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// KnowledgeFolderService exposes the operations the document-management UI
// (and the chat mention resolver) need to manipulate the directory tree of
// a document knowledge base. The interface mirrors WikiFolderService so the
// rename / move / delete semantics are identical across resources.
type KnowledgeFolderService interface {
	// ListChildFolders returns the direct children of parentID (parentID
	// empty = KB root) annotated with live document counts and a
	// has-children flag for the directory tree.
	ListChildFolders(ctx context.Context, kbID string, parentID string, pageTypes []string) ([]types.KnowledgeFolderNode, error)
	// ListFolderTree returns the full subtree rooted at parentID (parentID
	// empty = KB root). The tree is rendered by the document list page's
	// sidebar so it has to be deep enough to draw the entire hierarchy in
	// a single round-trip.
	ListFolderTree(ctx context.Context, kbID string, parentID string) ([]*types.KnowledgeFolderTreeNode, error)
	// CreateFolder creates a new (initially empty) folder under ParentID.
	// Name validation, parent existence, and the (kb, parent, name)
	// uniqueness invariant are enforced here so handlers can stay thin.
	CreateFolder(ctx context.Context, kbID string, tenantID uint64, parentID string, name string) (*types.KnowledgeFolder, error)
	// RenameOrMoveFolder renames and/or reparents a folder, then
	// recomputes the materialized path / depth of the entire subtree.
	// Cycles (moving into self / descendant) and sibling name collisions
	// are rejected before any write.
	RenameOrMoveFolder(ctx context.Context, kbID string, id string, newName string, newParentID string, moveParent bool) (*types.KnowledgeFolder, error)
	// DeleteFolder removes an empty folder. The UI must relocate
	// documents / child folders first; this is the non-destructive safety
	// guard.
	DeleteFolder(ctx context.Context, kbID string, id string) error
	// ResolveFolderIDs returns the supplied folder IDs plus every
	// descendant folder ID. Used by the chat @folder mention to expand
	// a folder pick into a complete document set.
	ResolveFolderIDs(ctx context.Context, kbID string, folderIDs []string, includeDescendants bool) ([]string, error)
	// ResolveKnowledgeIDs expands a folder pick (a list of folder IDs) into
	// the deduplicated list of knowledge IDs that fall inside those folders
	// (and, when includeDescendants is true, inside their subtrees). The
	// companion to KnowledgeService.ResolveFolderKnowledgeIDs, but
	// convenience here so the handler does not have to fan out into two
	// services.
	ResolveKnowledgeIDs(ctx context.Context, tenantID uint64, kbID string, folderIDs []string, includeDescendants bool) ([]string, error)
}
