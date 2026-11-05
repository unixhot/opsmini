package agent

import (
	"crypto/rand"
	"math/big"
	"os"
	"path/filepath"
	"strings"
)

// InstanceIDKey 是实例 ID 在面板 settings 表中的键名。
const InstanceIDKey = "agent.instance_id"

// instanceIDFileName 是实例 ID 在数据目录下的副本文件名（默认 /data/opsmini/agent_id）。
const instanceIDFileName = "agent_id"

// instanceIDPrefix 是实例 ID 的固定前缀，便于在日志、工单、全局搜索中识别来源。
// 形如 opsmini-7k3m9x2qv4ab：前缀 + 12 位小写 base36。
const instanceIDPrefix = "opsmini-"

const instanceIDRandLen = 12

// IDStore 是实例 ID 所需的设置读写能力，由 service.SettingService 实现。
// 定义为接口是为了让 agent 包不依赖 service/repository，便于单测。
type IDStore interface {
	Get(key string) (string, error)
	Set(kv map[string]string) error
}

// LoadOrCreateInstanceID 读取或生成面板实例 ID。
//
// 存储策略（DB 为准、文件为副本）：
//   - settings 表键 agent.instance_id 是权威值，有事务保证，面板二进制重装不影响；
//   - <dataDir>/agent_id 是副本，供运维脚本等外部工具直读，无需连 sqlite；
//   - 两者都存在但取值不同时以 DB 为准，并用 DB 值覆盖文件；
//   - 只有文件有值时用文件值回填 DB。
//
// 返回值 persisted 为 false 表示 DB 与文件都没写成功（例如只读根分区），
// 此时 ID 仅存在于内存，进程重启会变，调用方应打印警告。
func LoadOrCreateInstanceID(store IDStore, dataDir string) (id string, persisted bool) {
	filePath := filepath.Join(dataDir, instanceIDFileName)
	dbID := normalizeInstanceID(readFromStore(store))
	fileID := normalizeInstanceID(readIDFile(filePath))

	switch {
	case dbID != "":
		if fileID != dbID {
			_ = writeIDFile(filePath, dbID)
		}
		return dbID, true
	case fileID != "":
		dbOK := writeToStore(store, fileID)
		return fileID, dbOK
	}

	id = newInstanceID()
	dbOK := writeToStore(store, id)
	fileOK := writeIDFile(filePath, id) == nil
	return id, dbOK || fileOK
}

// IsInstanceID 判断字符串是否符合实例 ID 格式。
func IsInstanceID(v string) bool {
	v = strings.TrimSpace(v)
	return strings.HasPrefix(v, instanceIDPrefix) && len(v) == len(instanceIDPrefix)+instanceIDRandLen
}

// newInstanceID 生成 opsmini-<12 位小写 base36> 形式的实例 ID。
// 使用 crypto/rand 保证不可预测、无碰撞风险（36^12 ≈ 4.7e18）。
func newInstanceID() string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	max := big.NewInt(int64(len(alphabet)))
	buf := make([]byte, instanceIDRandLen)
	for i := range buf {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			// crypto/rand 在现代系统上不应失败；失败时退化为固定字符，仍保证格式合法。
			buf[i] = '0'
			continue
		}
		buf[i] = alphabet[n.Int64()]
	}
	return instanceIDPrefix + string(buf)
}

// normalizeInstanceID 清理空白，并丢弃不符合格式的历史值。
func normalizeInstanceID(v string) string {
	v = strings.TrimSpace(v)
	if !IsInstanceID(v) {
		return ""
	}
	return v
}

func readFromStore(store IDStore) string {
	if store == nil {
		return ""
	}
	v, err := store.Get(InstanceIDKey)
	if err != nil {
		return ""
	}
	return v
}

func writeToStore(store IDStore, id string) bool {
	if store == nil {
		return false
	}
	return store.Set(map[string]string{InstanceIDKey: id}) == nil
}

func readIDFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func writeIDFile(path, id string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(id+"\n"), 0o644)
}
