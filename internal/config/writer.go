package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// yamlMarshal 将配置序列化为 YAML（字段顺序由结构体定义保证稳定）。
// auth.password 统一以双引号输出：与文档示例一致，且避免特殊字符引发的引号风格不确定。
func yamlMarshal(cfg *Config) ([]byte, error) {
	var doc yaml.Node
	if err := doc.Encode(cfg); err != nil {
		return nil, err
	}
	if root := docRoot(&doc); root != nil {
		if auth := mapValue(root, "auth"); auth != nil {
			if pw := mapValue(auth, "password"); pw != nil {
				pw.Style = yaml.DoubleQuotedStyle
			}
		}
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		_ = enc.Close()
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeBack 以 yaml.Node 方式修改配置文件，最大限度保留用户已有的注释与结构。
// 通过“临时文件 + 原子替换”写入，避免热重载与写入产生竞争。
func writeBack(path string, mutate func(doc *yaml.Node) error) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("无法解析配置文件路径 %q: %w", path, err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}
	root := docRoot(&doc)

	if err := mutate(root); err != nil {
		return err
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		_ = enc.Close()
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	mode := os.FileMode(0o600)
	if st, err := os.Stat(abs); err == nil {
		mode = st.Mode()
	}
	tmp := abs + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), mode); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	if err := os.Rename(tmp, abs); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("替换配置文件失败: %w", err)
	}
	return nil
}

// docRoot 返回文档根映射节点（必要时补建）。
func docRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode {
		if len(doc.Content) == 0 {
			doc.Content = append(doc.Content, &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"})
		}
		return doc.Content[0]
	}
	return doc
}

// mapValue 查找映射节点中指定键的值节点。
func mapValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// setMapValue 设置（或新增）映射节点中的键值，值节点由 Go 值序列化而来。
func setMapValue(m *yaml.Node, key string, val any) error {
	var valueNode yaml.Node
	if err := valueNode.Encode(val); err != nil {
		return fmt.Errorf("序列化配置项 %q 失败: %w", key, err)
	}
	if existing := mapValue(m, key); existing != nil {
		*existing = valueNode
		return nil
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&valueNode,
	)
	return nil
}

// ensureMap 返回映射节点中指定键的值节点；若不存在则创建空映射。
func ensureMap(m *yaml.Node, key string) *yaml.Node {
	if v := mapValue(m, key); v != nil {
		if v.Kind != yaml.MappingNode {
			v.Kind = yaml.MappingNode
			v.Tag = "!!map"
			v.Value = ""
			v.Content = nil
		}
		return v
	}
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		node,
	)
	return node
}

// SetComposeProjects 更新配置文件中的 compose_projects 列表。
func SetComposeProjects(path string, projects []ComposeProject) error {
	if projects == nil {
		projects = []ComposeProject{}
	}
	return writeBack(path, func(root *yaml.Node) error {
		var seq yaml.Node
		if err := seq.Encode(projects); err != nil {
			return fmt.Errorf("序列化 compose_projects 失败: %w", err)
		}
		return setMapValue(root, "compose_projects", &seq)
	})
}

// SetAuth 更新配置文件中的登录用户名与密码（密码固定双引号输出）。
func SetAuth(path, username, password string) error {
	return writeBack(path, func(root *yaml.Node) error {
		auth := ensureMap(root, "auth")
		if err := setMapValue(auth, "username", username); err != nil {
			return err
		}
		if err := setMapValue(auth, "password", password); err != nil {
			return err
		}
		if pw := mapValue(auth, "password"); pw != nil {
			pw.Style = yaml.DoubleQuotedStyle
		}
		return nil
	})
}
