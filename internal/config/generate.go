package config

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
)

// 密码字符集：英文大小写、数字与特殊字符；排除易混淆字符（l/I/O/0/1）
// 以及引号、反斜杠、反引号、$ 等在 YAML/Shell 中易引发问题的字符。
const (
	lowerChars   = "abcdefghijkmnopqrstuvwxyz"
	upperChars   = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	digitChars   = "23456789"
	specialChars = "!@#%*+-_=?"
)

// GeneratePassword 使用 crypto/rand 生成指定位数的强随机密码，
// 保证至少包含大写、小写、数字与特殊字符各一个。
func GeneratePassword(length int) (string, error) {
	if length < MinPasswordLen {
		return "", fmt.Errorf("密码长度不得少于 %d 位", MinPasswordLen)
	}
	all := lowerChars + upperChars + digitChars + specialChars

	// 先确保四种字符类别各出现一次，其余从全集中随机补齐。
	classes := []string{lowerChars, upperChars, digitChars, specialChars}
	chars := make([]byte, 0, length)
	for _, class := range classes {
		ch, err := randomFrom(class)
		if err != nil {
			return "", err
		}
		chars = append(chars, ch)
	}
	for len(chars) < length {
		ch, err := randomFrom(all)
		if err != nil {
			return "", err
		}
		chars = append(chars, ch)
	}
	if err := shuffle(chars); err != nil {
		return "", err
	}
	return string(chars), nil
}

func randomFrom(set string) (byte, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(set))))
	if err != nil {
		return 0, fmt.Errorf("生成随机数失败: %w", err)
	}
	return set[n.Int64()], nil
}

func shuffle(b []byte) error {
	for i := len(b) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return fmt.Errorf("生成随机数失败: %w", err)
		}
		j := int(n.Int64())
		b[i], b[j] = b[j], b[i]
	}
	return nil
}

const generatedHeader = `# Docker Control Center (DCC) 配置文件
# 首次启动自动生成；修改后程序会自动热重载（无需重启）。
# 注意：auth.password 为登录凭据，请妥善保管本文件（Linux 下建议 chmod 600 config.yaml）。
# 所有相对路径均基于本文件所在目录解析。
`

// Generate 生成一份包含随机密码的全新配置文件（0600 权限）。
// 返回生成的配置与密码明文（供首次启动时提示与记录日志）。
func Generate(path string) (*Config, string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", fmt.Errorf("无法解析配置文件路径 %q: %w", path, err)
	}
	if _, err := os.Stat(abs); err == nil {
		return nil, "", fmt.Errorf("配置文件已存在: %s", abs)
	}
	if dir := filepath.Dir(abs); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, "", fmt.Errorf("创建配置目录失败: %w", err)
		}
	}

	password, err := GeneratePassword(16)
	if err != nil {
		return nil, "", err
	}

	cfg := NewDefault()
	cfg.Auth.Password = password
	cfg.Path = abs
	cfg.BaseDir = filepath.Dir(abs)

	body, err := yamlMarshal(cfg)
	if err != nil {
		return nil, "", fmt.Errorf("序列化配置失败: %w", err)
	}
	data := append([]byte(generatedHeader), body...)
	if err := os.WriteFile(abs, data, 0o600); err != nil {
		return nil, "", fmt.Errorf("写入配置文件失败: %w", err)
	}
	// Windows 下权限位不生效，Linux 下确保仅属主可读写。
	_ = os.Chmod(abs, 0o600)
	return cfg, password, nil
}

// MaskPassword 返回用于展示/日志的密码掩码。
func MaskPassword(p string) string {
	if p == "" {
		return ""
	}
	if len(p) <= 4 {
		return strings.Repeat("*", len(p))
	}
	return p[:2] + strings.Repeat("*", len(p)-4) + p[len(p)-2:]
}
