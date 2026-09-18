package ask

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// 本文件固定补取已有大结果的受控结果句柄（对应架构设计 v2 文档 7.2、7.3
// 分页与引用）。句柄绑定用户、家庭与查询条件，不允许更换查询身份或扩查其他
// 数据；过期时明确需重查，不扩查其他数据。

// resultHandleSalt：句柄校验码的固定盐。校验码用于防止误改与明显篡改，
// 不是密码学级别的授权；真实部署可替换为服务端密钥或持久化句柄。
const resultHandleSalt = "wechat-pet-result-handle-v1"

// ResultHandle：受控结果句柄。QueryHash 是查询条件（不含游标）的稳定指纹，
// Version 是数据集合版本，游标由调用方单独携带续读。
type ResultHandle struct {
	FamilyID  string `json:"family_id"`
	ToolName  string `json:"tool_name"`
	QueryHash string `json:"query_hash"`
	Version   string `json:"version"`
}

// checksum 计算句柄校验码。
func (h ResultHandle) checksum() string {
	sum := sha256.Sum256([]byte(h.FamilyID + "\x00" + h.ToolName + "\x00" + h.QueryHash + "\x00" + h.Version + "\x00" + resultHandleSalt))
	return hex.EncodeToString(sum[:])
}

// EncodeResultHandle 编码句柄（含校验码）。
func EncodeResultHandle(handle ResultHandle) string {
	payload, _ := json.Marshal(handle)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + handle.checksum()
}

// DecodeResultHandle 解码并校验句柄。格式非法或校验码不匹配返回错误。
func DecodeResultHandle(value string) (ResultHandle, error) {
	index := strings.LastIndex(value, ".")
	if index <= 0 || index == len(value)-1 {
		return ResultHandle{}, fmt.Errorf("ask: invalid result handle")
	}
	payload, err := base64.RawURLEncoding.DecodeString(value[:index])
	if err != nil {
		return ResultHandle{}, fmt.Errorf("ask: invalid result handle: %w", err)
	}
	var handle ResultHandle
	if err := json.Unmarshal(payload, &handle); err != nil {
		return ResultHandle{}, fmt.Errorf("ask: invalid result handle: %w", err)
	}
	if handle.checksum() != value[index+1:] {
		return ResultHandle{}, fmt.Errorf("ask: result handle checksum mismatch")
	}
	return handle, nil
}

// ValidateResultHandle 校验句柄与当前查询身份一致，防止更换家庭、工具或查询条件。
func ValidateResultHandle(handle ResultHandle, familyID, toolName, queryHash string) error {
	if handle.FamilyID != familyID {
		return fmt.Errorf("ask: result handle family mismatch")
	}
	if handle.ToolName != toolName {
		return fmt.Errorf("ask: result handle tool mismatch")
	}
	if handle.QueryHash != queryHash {
		return fmt.Errorf("ask: result handle query mismatch")
	}
	return nil
}
