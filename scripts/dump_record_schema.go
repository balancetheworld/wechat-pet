// 开发脚本：打印 record_array_v1 的机读 JSON Schema，供 strict 结构化输出验证脚本使用。
// 用法：go run ./scripts/dump_record_schema.go
package main

import (
	"encoding/json"
	"fmt"
	"os"

	askapp "github.com/balancetheworld/wechat-pet/internal/app/ask"
)

func main() {
	data, err := json.Marshal(askapp.RecordArraySchema())
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal record schema:", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}
