package asset

import (
	"crypto/rand"
	"encoding/hex"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	familyapp "github.com/balancetheworld/wechat-pet/internal/app/family"
	"github.com/balancetheworld/wechat-pet/internal/middleware"
	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
	jwtpkg "github.com/balancetheworld/wechat-pet/internal/pkg/jwt"
	"github.com/balancetheworld/wechat-pet/internal/pkg/response"
	fileservice "github.com/balancetheworld/wechat-pet/internal/service/file"
	"github.com/gin-gonic/gin"
)

const maxAvatarSize = 5 << 20
const maxMediaSize = 50 << 20

type Handler struct {
	service   *fileservice.Service
	family    familyapp.ActiveFamilyRepository
	uploadDir string
}

type uploadResponse struct {
	AssetID string `json:"asset_id"`
}

func NewHandler(service *fileservice.Service, family ...familyapp.ActiveFamilyRepository) *Handler {
	var repository familyapp.ActiveFamilyRepository
	if len(family) > 0 {
		repository = family[0]
	}
	return &Handler{service: service, family: repository}
}

func (h *Handler) SetUploadDir(value string) {
	h.uploadDir = strings.TrimSpace(value)
}

func (h *Handler) Upload(c *gin.Context) {
	if _, ok := middleware.GetCurrentUserID(c); !ok {
		response.Fail(c, appErrors.Unauthorized())
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxMediaSize+(1<<20))
	if err := c.Request.ParseMultipartForm(maxMediaSize); err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			response.Fail(c, appErrors.PayloadTooLarge("文件大小超出限制"))
			return
		}
		response.Fail(c, appErrors.InvalidParam("上传参数无效"))
		return
	}
	uploadType := c.PostForm("type")
	if !supportedType(uploadType) {
		response.Fail(c, appErrors.InvalidParam("不支持的文件类型"))
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			response.Fail(c, appErrors.PayloadTooLarge("文件大小超出限制"))
			return
		}
		response.Fail(c, appErrors.InvalidParam("请选择头像图片"))
		return
	}
	limit := int64(maxAvatarSize)
	if uploadType != "avatar" && uploadType != "pet_avatar" && uploadType != "pet_cover" {
		limit = maxMediaSize
	}
	if file.Size <= 0 || file.Size > limit {
		response.Fail(c, appErrors.InvalidParam("文件大小超出限制"))
		return
	}
	contentType := strings.ToLower(strings.TrimSpace(file.Header.Get("Content-Type")))
	extension := assetExtension(contentType, file.Filename, uploadType)
	if extension == "" {
		response.Fail(c, appErrors.InvalidParam("文件格式不支持"))
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
	userID, _ := middleware.GetCurrentUserID(c)
	familyID := ""
	if h.family != nil {
		if summary, familyErr := h.family.GetActiveFamilySummary(c.Request.Context(), userID); familyErr != nil {
			response.Fail(c, appErrors.Internal(familyErr))
			return
		} else if summary != nil {
			familyID = summary.ID
		}
	}
	if _, err := h.service.UploadForOwner(c.Request.Context(), assetID, content, file.Size, contentType, fileservice.Asset{ID: assetID, UserID: userID, FamilyID: familyID, Type: uploadType}); err != nil {
		response.Fail(c, appErrors.Internal(err))
		return
	}
	response.Success(c, uploadResponse{AssetID: assetID})
}

func (h *Handler) Download(c *gin.Context) {
	if h.uploadDir == "" {
		response.Fail(c, appErrors.NotFound("资源不存在"))
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		response.Fail(c, appErrors.Unauthorized())
		return
	}
	assetID := strings.TrimSpace(c.Param("asset_id"))
	asset, err := h.service.Get(c.Request.Context(), assetID)
	if err != nil {
		response.Fail(c, appErrors.NotFound("资源不存在"))
		return
	}
	if asset.UserID != userID && !h.belongsToActiveFamily(c, userID, asset.FamilyID) {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	if filepath.Base(assetID) != assetID {
		response.Fail(c, appErrors.NotFound("资源不存在"))
		return
	}
	path := filepath.Join(h.uploadDir, assetID)
	if _, err := os.Stat(path); err != nil {
		response.Fail(c, appErrors.NotFound("资源不存在"))
		return
	}
	c.Header("X-Content-Type-Options", "nosniff")
	http.ServeFile(c.Writer, c.Request, path)
}

func (h *Handler) belongsToActiveFamily(c *gin.Context, userID, assetFamilyID string) bool {
	if h.family == nil || assetFamilyID == "" {
		return false
	}
	summary, err := h.family.GetActiveFamilySummary(c.Request.Context(), userID)
	return err == nil && summary != nil && summary.ID == assetFamilyID
}

func currentUserID(c *gin.Context) (string, bool) {
	if userID, ok := middleware.GetCurrentUserID(c); ok {
		return userID, true
	}
	token := strings.TrimSpace(c.Query("access_token"))
	if token == "" {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			token = parts[1]
		}
	}
	if token == "" {
		return "", false
	}
	value, ok := c.Get("asset_token_signer")
	if !ok {
		return "", false
	}
	signer, ok := value.(*jwtpkg.Signer)
	if !ok {
		return "", false
	}
	claims, err := signer.Verify(token)
	if err != nil {
		return "", false
	}
	return claims.UserID, true
}

func supportedType(value string) bool {
	switch value {
	case "avatar", "pet_avatar", "pet_cover", "birthday_photo", "birthday_video", "growth_image", "calendar_image":
		return true
	}
	return false
}

func assetExtension(contentType, fileName, uploadType string) string {
	if uploadType == "birthday_video" {
		ext := strings.ToLower(filepath.Ext(fileName))
		if strings.HasPrefix(contentType, "video/") || ext == ".mp4" || ext == ".mov" {
			if ext == "" {
				ext = ".mp4"
			}
			return ext
		}
		return ""
	}
	return avatarExtension(contentType, fileName)
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
