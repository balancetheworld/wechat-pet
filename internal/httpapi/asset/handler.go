package asset

import (
	"crypto/rand"
	"encoding/hex"
	"mime"
	"path/filepath"
	"strings"

	"github.com/balancetheworld/wechat-pet/internal/middleware"
	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
	"github.com/balancetheworld/wechat-pet/internal/pkg/response"
	fileservice "github.com/balancetheworld/wechat-pet/internal/service/file"
	"github.com/gin-gonic/gin"
)

const maxAvatarSize = 5 << 20

type Handler struct{ service *fileservice.Service }

type uploadResponse struct {
	AssetID string `json:"asset_id"`
}

func NewHandler(service *fileservice.Service) *Handler { return &Handler{service: service} }

func (h *Handler) Upload(c *gin.Context) {
	if _, ok := middleware.GetCurrentUserID(c); !ok {
		response.Fail(c, appErrors.Unauthorized())
		return
	}
	if c.PostForm("type") != "avatar" {
		response.Fail(c, appErrors.InvalidParam("仅支持上传头像"))
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, appErrors.InvalidParam("请选择头像图片"))
		return
	}
	if file.Size <= 0 || file.Size > maxAvatarSize {
		response.Fail(c, appErrors.InvalidParam("头像图片不能超过 5MB"))
		return
	}
	contentType := strings.ToLower(strings.TrimSpace(file.Header.Get("Content-Type")))
	extension := avatarExtension(contentType, file.Filename)
	if extension == "" {
		response.Fail(c, appErrors.InvalidParam("头像仅支持 JPG、PNG 或 WEBP 图片"))
		return
	}
	content, err := file.Open()
	if err != nil {
		response.Fail(c, appErrors.Internal(err))
		return
	}
	defer content.Close()
	assetID, err := newAssetID(extension)
	if err != nil {
		response.Fail(c, appErrors.Internal(err))
		return
	}
	if _, err := h.service.Upload(c.Request.Context(), assetID, content, file.Size, contentType); err != nil {
		response.Fail(c, appErrors.Internal(err))
		return
	}
	response.Success(c, uploadResponse{AssetID: assetID})
}

func avatarExtension(contentType, fileName string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	}
	extension := strings.ToLower(filepath.Ext(fileName))
	if mime.TypeByExtension(extension) == "image/jpeg" || extension == ".png" || extension == ".webp" {
		return extension
	}
	return ""
}

func newAssetID(extension string) (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]) + extension, nil
}
