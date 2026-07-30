package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// KnowledgeFolderRepository sentinel errors. They are declared here so the
// service layer can branch on them without depending on the concrete
// repository package.
var (
	// ErrKnowledgeFolderNotFound is returned when a knowledge folder is
	// missing (deleted, wrong KB, or simply never existed).
	ErrKnowledgeFolderNotFound = errNotFound("knowledge folder not found")
	// ErrKnowledgeFolderConflict is returned when a sibling folder with the
	// same name already exists under the same parent.
	ErrKnowledgeFolderConflict = errConflict("knowledge folder name conflict")
	// ErrKnowledgeFolderNotEmpty is returned when a folder still has a live
	// document or child folder at the instant an atomic delete is attempted.
	ErrKnowledgeFolderNotEmpty = errNotEmpty("knowledge folder is not empty")
)

// errNotFound / errConflict / errNotEmpty are private sentinel types used to
// wrap errors with stable identity. We use strings so the service layer can
// compare with errors.Is.
type errNotFound string

func (e errNotFound) Error() string { return string(e) }

type errConflict string

func (e errConflict) Error() string { return string(e) }

type errNotEmpty string

func (e errNotEmpty) Error() string { return string(e) }

// KnowledgeFolderRepository defines persistence operations for the knowledge
// folder hierarchy. The methods mirror the wiki folder repository verbatim
// so the service layer can re-use the same rename/move/delete algorithms
// the wiki already trusts in production.
type KnowledgeFolderRepository interface {
	// CreateFolder inserts a new folder row.
	CreateFolder(ctx context.Context, folder *types.KnowledgeFolder) error
	// GetFolderByID returns a single folder or ErrKnowledgeFolderNotFound.
	GetFolderByID(ctx context.Context, kbID string, id string) (*types.KnowledgeFolder, error)
	// GetChildFolderByName returns the direct child of parentID with the
	// supplied name. Used by the (kb, parent, name) unique-constraint
	// pre-check before create / rename / move.
	GetChildFolderByName(ctx context.Context, kbID string, parentID string, name string) (*types.KnowledgeFolder, error)
	// ListChildFolders returns the direct children of parentID (parentID
	// empty = KB root level), ordered by (sort_order, name).
	ListChildFolders(ctx context.Context, kbID string, parentID string) ([]*types.KnowledgeFolder, error)
	// ListAllFolders returns every folder in the KB, ordered by (depth,
	// path). Used by the rename / move path to recompute the materialized
	// path of the entire subtree in a single read.
	ListAllFolders(ctx context.Context, kbID string) ([]*types.KnowledgeFolder, error)
	// UpdateFolder updates parent_id, name, path, depth, sort_order on
	// the row identified by the receiver's ID. Returns
	// ErrKnowledgeFolderNotFound when no row matches.
	UpdateFolder(ctx context.Context, folder *types.KnowledgeFolder) error
	// DeleteFolder atomically soft-deletes an empty folder. Returns
	// ErrKnowledgeFolderNotEmpty when the folder still has live documents
	// or child folders at the instant of delete, and
	// ErrKnowledgeFolderNotFound when the row is already gone.
	DeleteFolder(ctx context.Context, kbID string, id string) error
	// ListDescendantFolderIDs returns the input folder IDs plus every
	// descendant folder ID reachable through the parent chain. Used by
	// the chat-resolution expansion to build a "folder + subtree" set
	// for the (knowledge_base_id, folder_id) IN (...) clause.
	ListDescendantFolderIDs(ctx context.Context, kbID string, folderIDs []string) ([]string, error)
}
