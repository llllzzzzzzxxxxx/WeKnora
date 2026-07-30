package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
)

// KnowledgeFolderHandler processes HTTP requests for document knowledge base folder management.
// The routes mirror WikiFolder routes but are scoped to document KBs and managed
// by a dedicated KnowledgeFolderService (independent of wiki_folders).
type KnowledgeFolderHandler struct {
	folderService  interfaces.KnowledgeFolderService
	knowledgeSvc   interfaces.KnowledgeService
	kbService      interfaces.KnowledgeBaseService
	kbShareService interfaces.KBShareService
}

// NewKnowledgeFolderHandler constructs a handler for the document folder API.
func NewKnowledgeFolderHandler(
	folderService interfaces.KnowledgeFolderService,
	knowledgeSvc interfaces.KnowledgeService,
	kbService interfaces.KnowledgeBaseService,
	kbShareService interfaces.KBShareService,
) *KnowledgeFolderHandler {
	return &KnowledgeFolderHandler{
		folderService:  folderService,
		knowledgeSvc:   knowledgeSvc,
		kbService:      kbService,
		kbShareService: kbShareService,
	}
}

// resolveDocumentKB validates that the :kb_id belongs to the caller and is a
// document-type knowledge base. FAQ and Wiki KBs are rejected with 400.
func (h *KnowledgeFolderHandler) resolveDocumentKB(c *gin.Context) (string, uint64, error) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("kb_id"))
	tenantID := c.GetUint64(types.TenantIDContextKey.String())

	if kbID == "" {
		return "", 0, errors.NewBadRequestError("knowledge base id is required")
	}
	if tenantID == 0 {
		return "", 0, errors.NewUnauthorizedError("unauthorized")
	}

	kb, err := h.kbService.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		if err == repository.ErrKnowledgeBaseNotFound {
			return "", 0, errors.NewNotFoundError("knowledge base not found")
		}
		return "", 0, errors.NewInternalServerError(err.Error())
	}

	if kb.Type != types.KnowledgeBaseTypeDocument {
		return "", 0, errors.NewBadRequestError("folders are only supported for document knowledge bases")
	}

	return kbID, tenantID, nil
}

// ListChildFolders godoc
// @Summary      List folder children
// @Description  List direct child folders of a parent folder inside a document KB. Omit parent_id or pass "" to list root-level folders.
// @Tags         KnowledgeFolders
// @Produce      json
// @Param        kb_id     path  string  true   "Knowledge base ID"
// @Param        parent_id query string  false  "Parent folder ID (empty = KB root)"
// @Success      200       {object} types.KnowledgeFolderListResponse
// @Failure      400       {object} errors.AppError
// @Failure      404       {object} errors.AppError
// @Security     Bearer
// @Router       /knowledge-bases/{kb_id}/folders [get]
func (h *KnowledgeFolderHandler) ListChildFolders(c *gin.Context) {
	ctx := c.Request.Context()
	kbID, tenantID, err := h.resolveDocumentKB(c)
	if err != nil {
		c.Error(err)
		return
	}

	parentID := strings.TrimSpace(c.Query("parent_id"))
	pageTypes := []string{}

	folders, err := h.folderService.ListChildFolders(ctx, kbID, parentID, pageTypes)
	if err != nil {
		logger.Errorf(ctx, "ListChildFolders failed kb=%s parent=%s: %v", kbID, parentID, err)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	if folders == nil {
		folders = []types.KnowledgeFolderNode{}
	}

	_ = context.WithValue(ctx, types.TenantIDContextKey, tenantID)

	c.JSON(http.StatusOK, types.KnowledgeFolderListResponse{
		ParentID: parentID,
		Folders:  folders,
	})
}

// ListFolderTree godoc
// @Summary      List full folder tree
// @Description  Returns the complete subtree rooted at parent_id (or KB root if omitted). Used by the document list sidebar.
// @Tags         KnowledgeFolders
// @Produce      json
// @Param        kb_id     path  string  true   "Knowledge base ID"
// @Param        parent_id query string  false  "Root of subtree (empty = full KB tree)"
// @Success      200       {object} types.KnowledgeFolderTree
// @Failure      400       {object} errors.AppError
// @Failure      404       {object} errors.AppError
// @Security     Bearer
// @Router       /knowledge-bases/{kb_id}/folders/tree [get]
func (h *KnowledgeFolderHandler) ListFolderTree(c *gin.Context) {
	ctx := c.Request.Context()
	kbID, _, err := h.resolveDocumentKB(c)
	if err != nil {
		c.Error(err)
		return
	}

	parentID := strings.TrimSpace(c.Query("parent_id"))
	tree, err := h.folderService.ListFolderTree(ctx, kbID, parentID)
	if err != nil {
		logger.Errorf(ctx, "ListFolderTree failed kb=%s parent=%s: %v", kbID, parentID, err)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	c.JSON(http.StatusOK, types.KnowledgeFolderTree{
		KnowledgeBaseID: kbID,
		Root:            tree,
	})
}

// CreateFolder godoc
// @Summary      Create a folder
// @Description  Creates a new (initially empty) directory node inside a document KB.
// @Tags         KnowledgeFolders
// @Accept       json
// @Produce      json
// @Param        kb_id    path  string  true  "Knowledge base ID"
// @Param        request  body  types.KnowledgeFolderCreateRequest  true  "Folder data"
// @Success      201      {object} types.KnowledgeFolder
// @Failure      400      {object} errors.AppError
// @Failure      409      {object} errors.AppError  "name conflict"
// @Security     Bearer
// @Router       /knowledge-bases/{kb_id}/folders [post]
func (h *KnowledgeFolderHandler) CreateFolder(c *gin.Context) {
	ctx := c.Request.Context()
	kbID, tenantID, err := h.resolveDocumentKB(c)
	if err != nil {
		c.Error(err)
		return
	}

	var req types.KnowledgeFolderCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}

	folder, err := h.folderService.CreateFolder(ctx, kbID, tenantID, req.ParentID, req.Name)
	if err != nil {
		if strings.Contains(err.Error(), "conflict") || strings.Contains(err.Error(), "duplicate") {
			c.Error(errors.NewConflictError(err.Error()))
			return
		}
		if strings.Contains(err.Error(), "not found") {
			c.Error(errors.NewNotFoundError(err.Error()))
			return
		}
		logger.Errorf(ctx, "CreateFolder failed kb=%s: %v", kbID, err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	logger.Infof(ctx, "Created folder id=%s kb=%s name=%s", folder.ID, kbID, folder.Name)
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": folder})
}

// UpdateFolder godoc
// @Summary      Update (rename / move) a folder
// @Description  Renames and/or reparents a folder. MoveParent must be true to trigger reparenting. Moving into self or a descendant is rejected.
// @Tags         KnowledgeFolders
// @Accept       json
// @Produce      json
// @Param        kb_id     path  string  true  "Knowledge base ID"
// @Param        folder_id path  string  true  "Folder ID"
// @Param        request   body  types.KnowledgeFolderUpdateRequest  true  "Update data"
// @Success      200       {object} types.KnowledgeFolder
// @Failure      400      {object} errors.AppError
// @Failure      404      {object} errors.AppError
// @Failure      409      {object} errors.AppError  "name conflict"
// @Security     Bearer
// @Router       /knowledge-bases/{kb_id}/folders/{folder_id} [put]
func (h *KnowledgeFolderHandler) UpdateFolder(c *gin.Context) {
	ctx := c.Request.Context()
	kbID, _, err := h.resolveDocumentKB(c)
	if err != nil {
		c.Error(err)
		return
	}

	folderID := strings.TrimSpace(c.Param("folder_id"))
	if folderID == "" {
		c.Error(errors.NewBadRequestError("folder_id is required"))
		return
	}

	var req types.KnowledgeFolderUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}

	folder, err := h.folderService.RenameOrMoveFolder(ctx, kbID, folderID, req.Name, req.ParentID, req.MoveParent)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.Error(errors.NewNotFoundError(err.Error()))
			return
		}
		if strings.Contains(err.Error(), "conflict") || strings.Contains(err.Error(), "duplicate") {
			c.Error(errors.NewConflictError(err.Error()))
			return
		}
		if strings.Contains(err.Error(), "cycle") ||
			strings.Contains(err.Error(), "descendant") ||
			strings.Contains(err.Error(), "itself") {
			c.Error(errors.NewBadRequestError(err.Error()))
			return
		}
		logger.Errorf(ctx, "UpdateFolder failed kb=%s folder=%s: %v", kbID, folderID, err)
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	logger.Infof(ctx, "Updated folder id=%s kb=%s", folder.ID, kbID)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": folder})
}

// DeleteFolder godoc
// @Summary      Delete a folder
// @Description  Soft-deletes an empty folder. Fails 409 if the folder has live documents or child folders.
// @Tags         KnowledgeFolders
// @Produce      json
// @Param        kb_id     path  string  true  "Knowledge base ID"
// @Param        folder_id path  string  true  "Folder ID"
// @Success      200      {object} map[string]interface{}
// @Failure      400      {object} errors.AppError
// @Failure      404      {object} errors.AppError
// @Failure      409      {object} errors.AppError  "folder not empty"
// @Security     Bearer
// @Router       /knowledge-bases/{kb_id}/folders/{folder_id} [delete]
func (h *KnowledgeFolderHandler) DeleteFolder(c *gin.Context) {
	ctx := c.Request.Context()
	kbID, _, err := h.resolveDocumentKB(c)
	if err != nil {
		c.Error(err)
		return
	}

	folderID := strings.TrimSpace(c.Param("folder_id"))
	if folderID == "" {
		c.Error(errors.NewBadRequestError("folder_id is required"))
		return
	}

	if err := h.folderService.DeleteFolder(ctx, kbID, folderID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.Error(errors.NewNotFoundError(err.Error()))
			return
		}
		if strings.Contains(err.Error(), "not empty") {
			c.Error(errors.NewConflictError(err.Error()))
			return
		}
		logger.Errorf(ctx, "DeleteFolder failed kb=%s folder=%s: %v", kbID, folderID, err)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Deleted folder id=%s kb=%s", folderID, kbID)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "folder deleted"})
}

// MoveKnowledgeToFolder godoc
// @Summary      Move documents into a folder
// @Description  Relocates one or more document knowledge items into the specified folder inside the same KB. Pass folder_id = "" to move to KB root.
// @Tags         KnowledgeFolders
// @Accept       json
// @Produce      json
// @Param        kb_id   path  string  true  "Knowledge base ID"
// @Param        request body  types.MoveKnowledgeToFolderRequest  true  "Move request"
// @Success      200    {object} types.MoveKnowledgeToFolderResponse
// @Failure      400    {object} errors.AppError
// @Failure      404    {object} errors.AppError
// @Security     Bearer
// @Router       /knowledge-bases/{kb_id}/folders/move-knowledge [post]
func (h *KnowledgeFolderHandler) MoveKnowledgeToFolder(c *gin.Context) {
	ctx := c.Request.Context()
	kbID, tenantID, err := h.resolveDocumentKB(c)
	if err != nil {
		c.Error(err)
		return
	}

	var req types.MoveKnowledgeToFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}

	// Override KBID from URL so the request body doesn't have to mirror it.
	req.KBID = kbID

	acknowledged, err := h.knowledgeSvc.MoveKnowledgeToFolder(ctx, tenantID, kbID, req.KnowledgeIDs, req.FolderID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.Error(errors.NewNotFoundError("knowledge folder not found"))
			return
		}
		if strings.Contains(err.Error(), "not belong") {
			c.Error(errors.NewBadRequestError(err.Error()))
			return
		}
		logger.Errorf(ctx, "MoveKnowledgeToFolder failed kb=%s: %v", kbID, err)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Moved %d documents to folder %s in kb=%s", len(acknowledged), req.FolderID, kbID)
	c.JSON(http.StatusOK, types.MoveKnowledgeToFolderResponse{
		Acknowledged: acknowledged,
		FolderID:     req.FolderID,
	})
}
