package v1

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// AppCategoryHandler app category handler.
type AppCategoryHandler struct {
	svc *service.AppCategoryService
}

// NewAppCategoryHandler constructor.
func NewAppCategoryHandler(svc *service.AppCategoryService) *AppCategoryHandler {
	return &AppCategoryHandler{svc: svc}
}

// List GET /app-categories - category list.
func (h *AppCategoryHandler) List(c *gin.Context) {
	list, err := h.svc.List()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

type categoryReq struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// Create POST /app-categories - create a category.
func (h *AppCategoryHandler) Create(c *gin.Context) {
	var req categoryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	cat, err := h.svc.Create(req.Key, req.Name)
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, cat)
}

// Update PUT /app-categories/:id - update a category.
func (h *AppCategoryHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "invalid id")
		return
	}
	var req categoryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	if err := h.svc.Update(uint(id), req.Key, req.Name); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, nil)
}

// Delete DELETE /app-categories/:id - delete a category.
func (h *AppCategoryHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "invalid id")
		return
	}
	if err := h.svc.Delete(uint(id)); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, nil)
}
