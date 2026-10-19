// Package cli 提供 ds-search 命令行工具的实现:
// 子命令分发、API Key 配置链与面向 AI 的输出格式。
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// APIKeyEnv 环境变量名。
const APIKeyEnv = "DEEPSEEK_API_KEY"

// AppDir 配置目录名(挂在各平台的用户配置根目录下)。
const AppDir = "ds-search"

// configFileName 配置文件名。
const configFileName = "config.json"

// Config 持久化配置(存于用户配置目录)。
type Config struct {
	APIKey string `json:"apiKey,omitempty"`
}

// ConfigDir 返回配置目录(os.UserConfigDir + AppDir)。
// windows: %APPDATA%\ds-search
// darwin:  ~/Library/Application Support/ds-search
// linux:   $XDG_CONFIG_HOME or ~/.config + /ds-search
func ConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, AppDir), nil
}

// ConfigPath 返回配置文件完整路径。
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

// LoadConfig 读取配置文件;文件不存在时返回零值 Config(nil)。
// 读入前剥离 UTF-8 BOM — 手工编辑配置时编辑器可能写入 BOM,会让 JSON 解析直接失败。
func LoadConfig() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &c, nil
}

// SaveConfig 写入配置文件(0600,含父目录创建)。
func SaveConfig(c *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}

// KeySource API Key 的来源。
type KeySource string

// Key 来源优先级:命令行参数 > 环境变量 > 配置文件。
const (
	KeySourceFlag   KeySource = "flag"
	KeySourceEnv    KeySource = "env"
	KeySourceConfig KeySource = "config"
)

// ResolveKey 按 flag > env > config 的顺序解析 API Key。
// 各来源统一经 cleanKeyInput 清洗(BOM、空白),与保存通道同规;
// 返回 key(可能为空)与来源;配置文件读取失败时报错。
func ResolveKey(flagValue string) (key string, source KeySource, err error) {
	if v := cleanKeyInput(flagValue); v != "" {
		return v, KeySourceFlag, nil
	}
	if v := cleanKeyInput(os.Getenv(APIKeyEnv)); v != "" {
		return v, KeySourceEnv, nil
	}
	cfg, err := LoadConfig()
	if err != nil {
		return "", "", err // 配置文件损坏要让用户知道,而不是静默跳过
	}
	if v := cleanKeyInput(cfg.APIKey); v != "" {
		return v, KeySourceConfig, nil
	}
	return "", "", nil
}

// MaskKey 掩码展示:sk-0123456789abcdef → sk-01…cdef。
func MaskKey(k string) string {
	if k == "" {
		return "(not set)"
	}
	const keep = 4
	if len(k) <= 8+keep {
		return strings.Repeat("*", len(k))
	}
	return k[:8] + "…" + k[len(k)-keep:]
}

// RunConfig `ds-search config path` — 打印配置文件路径。
// 沉到 cli 包以复用统一错误出口(writeErr/classify),与其它子命令同层。
func RunConfig(stdout, stderr io.Writer, args []string) int {
	if len(args) == 1 && args[0] == "path" {
		path, err := ConfigPath()
		if err != nil {
			return writeErr(stderr, err)
		}
		fmt.Fprintln(stdout, path)
		return ExitOK
	}
	fmt.Fprintln(stderr, "usage: ds-search config path")
	return ExitUsage
}
