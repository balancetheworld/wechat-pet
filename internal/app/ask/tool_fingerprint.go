package ask

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// 本文件固定工具调用的参数规范化与指纹（对应架构设计 v2 文档 7 开头）。
// 指纹是重复识别依据，不是跨用户共享结果或重复写入的授权。

// CanonicalJSON 生成参数的规范化 JSON：对象键递归排序、数组顺序保留，
// 并区分字符串、数字、布尔值和 null。键顺序不同但语义相同的参数得到相同输出。
func CanonicalJSON(value any) (string, error) {
	var builder strings.Builder
	if err := writeCanonical(&builder, value); err != nil {
		return "", err
	}
	return builder.String(), nil
}

func writeCanonical(builder *strings.Builder, value any) error {
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		builder.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				builder.WriteByte(',')
			}
			keyBytes, _ := json.Marshal(key)
			builder.Write(keyBytes)
			builder.WriteByte(':')
			if err := writeCanonical(builder, v[key]); err != nil {
				return err
			}
		}
		builder.WriteByte('}')
		return nil
	case []any:
		builder.WriteByte('[')
		for index, item := range v {
			if index > 0 {
				builder.WriteByte(',')
			}
			if err := writeCanonical(builder, item); err != nil {
				return err
			}
		}
		builder.WriteByte(']')
		return nil
	case string:
		encoded, _ := json.Marshal(v)
		builder.Write(encoded)
		return nil
	case bool:
		if v {
			builder.WriteString("true")
		} else {
			builder.WriteString("false")
		}
		return nil
	case nil:
		builder.WriteString("null")
		return nil
	case json.Number:
		builder.WriteString(v.String())
		return nil
	case float64:
		builder.WriteString(formatJSONNumber(v))
		return nil
	case float32:
		builder.WriteString(formatJSONNumber(float64(v)))
		return nil
	case int:
		builder.WriteString(strconv.Itoa(v))
		return nil
	case int64:
		builder.WriteString(strconv.FormatInt(v, 10))
		return nil
	case json.RawMessage:
		return writeCanonicalRaw(builder, v)
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		builder.Write(encoded)
		return nil
	}
}

// writeCanonicalRaw 将已编码的 JSON 片段反序列化后重新规范化，保证嵌套
// 结构（包括来自 json.RawMessage 的参数）也参与键排序。
func writeCanonicalRaw(builder *strings.Builder, raw json.RawMessage) error {
	if len(raw) == 0 {
		builder.WriteString("null")
		return nil
	}
	var decoded any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	return writeCanonical(builder, decoded)
}

// formatJSONNumber 用与 encoding/json 一致的数值格式输出 float64，并区分
// 整数与小数（1 与 1.0 语义上等同数字，统一为 1）。
func formatJSONNumber(value float64) string {
	if value == float64(int64(value)) {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'g', -1, 64)
}

// ToolFingerprint 计算一次工具调用的参数指纹：
// 工具名 + 工具/Schema 版本 + canonical JSON 参数 → SHA-256。
func ToolFingerprint(toolName, toolVersion string, arguments any) (string, error) {
	canonical, err := CanonicalJSON(arguments)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(toolName))
	h.Write([]byte{0})
	h.Write([]byte(toolVersion))
	h.Write([]byte{0})
	h.Write([]byte(canonical))
	return hex.EncodeToString(h.Sum(nil)), nil
}
