package types

import (
	"time"

	"gorm.io/gorm"
)

// KnowledgeFolderRootID is the sentinel value used throughout the codebase
// to denote the KB root. Folders are stored with parent_id == "" and
// documents are stored with folder_id == "" to indicate they live at the
// root of their knowledge base. Centralizing the value avoids magic-string
// drift across the repository / service / handler layers.
const KnowledgeFolderRootID = ""

// KnowledgeFolder is a first-class directory node inside a document knowledge
// base. A folder can exist independently of any document (the user can
// scaffold the tree before filing documents into it), and documents are
// attached to a folder via knowledges.folder_id. The hierarchy is encoded
// as an adjacency list (parent_id), with a materialized /"-joined Path
// kept for cheap display and sort. Renaming / moving a folder updates the
// path / depth of the entire subtree in a single transaction so the
// document-folder retrieval flow can match by path prefix without scanning
// the entire folder table.
type KnowledgeFolder struct {
	ID              string         `json:"id"                  gorm:"type:varchar(36);primaryKey"`
	TenantID        uint64         `json:"tenant_id"           gorm:"index"`
	KnowledgeBaseID string         `json:"knowledge_base_id"   gorm:"type:varchar(36);index"`
	ParentID        string         `json:"parent_id"           gorm:"column:parent_id;type:varchar(36);index;default:''"`
	Name            string         `json:"name"                gorm:"type:varchar(255)"`
	Path            string         `json:"path"                gorm:"type:varchar(1024)"`
	Depth           int            `json:"depth"               gorm:"default:0"`
	SortOrder       int            `json:"sort_order"          gorm:"default:0"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `json:"deleted_at"          gorm:"index"`
}

// TableName pins the database table name so GORM does not pluralize.
func (KnowledgeFolder) TableName() string {
	return "knowledge_folders"
}

// KnowledgeFolderNode is what the UI renders for a single directory entry.
// It carries the live document count underneath the folder so the browser
// can show "12 documents" without a second round-trip, plus a has-children
// flag so the expand affordance can be drawn from the same payload.
type KnowledgeFolderNode struct {
	KnowledgeFolder
	DocumentCount int64 `json:"document_count"`
	HasChildren   bool  `json:"has_children"`
}

// KnowledgeFolderListResponse is the payload for listing the direct children
// of a folder (parent_id = "" = KB root level).
type KnowledgeFolderListResponse struct {
	ParentID string                 `json:"parent_id"`
	Folders  []KnowledgeFolderNode `json:"folders"`
}

// KnowledgeFolderCreateRequest creates a new (initially empty) folder under
// the supplied parent. ParentID is "" to create a root-level folder.
type KnowledgeFolderCreateRequest struct {
	ParentID string `json:"parent_id"`
	Name     string `json:"name"`
}

// KnowledgeFolderUpdateRequest renames and/or reparents a folder. MoveParent
// is a separate flag so a pure rename does not have to re-send the (possibly
// root) parent and risk an accidental move.
type KnowledgeFolderUpdateRequest struct {
	Name       string `json:"name,omitempty"`
	ParentID   string `json:"parent_id,omitempty"`
	MoveParent bool   `json:"move_parent,omitempty"`
}

// KnowledgeFolderTreeNode is a recursive entry of the directory tree that the
// document list page renders in its sidebar. Each level carries the same
// counts as KnowledgeFolderNode so the UI can build stagger-load UIs without
// re-fetching every folder's children.
type KnowledgeFolderTreeNode struct {
	KnowledgeFolderNode
	Children []*KnowledgeFolderTreeNode `json:"children,omitempty"`
}

// MoveKnowledgeToFolderRequest relocates one or more KB documents into a
// folder. folder_id = "" means "move to the KB root" (out of any folder).
// KBID is required so the handler can validate the bound KB once and the
// service can batch-update rows without re-resolving tenant scope.
type MoveKnowledgeToFolderRequest struct {
	KBID         string   `json:"kb_id"          binding:"required"`
	KnowledgeIDs []string `json:"knowledge_ids"  binding:"required,min=1"`
	FolderID     string   `json:"folder_id"`
}

// MoveKnowledgeToFolderResponse is the response payload for the move-into-
// folder operation. Acknowledged carries the IDs that were actually moved
// (after the bounded-KB "belongs to this KB" check) so the UI can clear
// exactly those rows from its current view.
type MoveKnowledgeToFolderResponse struct {
	Acknowledged []string `json:"acknowledged"`
	FolderID     string   `json:"folder_id"`
}

// KnowledgeFolderTree is the full subtree rendered when the document list
// page loads. It is rooted at the KB root (no synthetic node) so the UI can
// pass the children array straight to its tree component.
type KnowledgeFolderTree struct {
	KnowledgeBaseID string                     `json:"knowledge_base_id"`
	Root            []*KnowledgeFolderTreeNode `json:"root"`
}

// FolderScopeQuery is the input shape for chat-side folder expansion. The
// chat path passes a small slice of folder IDs plus a flag that mirrors the
// KnowledgeListFilter.FolderScope semantics, so the service can resolve
// "folder X and its subtree" in one repository call.
type FolderScopeQuery struct {
	// FolderIDs are the leaf-level folders the user picked in the chat input.
	// The resolver walks knowledge_folders to find every descendant; an ID
	// that does not resolve (stale mention, deleted folder) is silently
	// dropped so the rest of the expansion can still succeed.
	FolderIDs []string
	// IncludeDescendants mirrors KnowledgeListFilter.FolderScopeTree. When
	// true, the resolver pulls every descendant folder and unions the
	// document IDs; when false, only the explicitly given folders are
	// considered.
	IncludeDescendants bool
}
