package docker

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestValidateProjectDir(t *testing.T) {
	dir := t.TempDir()
	if _, err := ValidateProjectDir(dir); err == nil {
		t.Fatal("空目录应报错")
	}

	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	name, err := ValidateProjectDir(dir)
	if err != nil {
		t.Fatalf("存在 compose 文件应通过: %v", err)
	}
	if name != "docker-compose.yml" {
		t.Fatalf("文件名错误: %s", name)
	}

	if _, err := ValidateProjectDir(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("不存在的目录应报错")
	}
}

func TestNormalizeProjectName(t *testing.T) {
	cases := map[string]string{
		"/opt/My App":     "my-app",
		"/opt/demo_web":   "demo_web",
		"/opt/a.b.c":      "a-b-c",
		"/opt/---x---":    "x",
		"/opt/":           "opt",
	}
	for in, want := range cases {
		if got := NormalizeProjectName(in); got != want {
			t.Fatalf("NormalizeProjectName(%q) = %q, 期望 %q", in, got, want)
		}
	}
}

func TestActionArgs(t *testing.T) {
	update, err := ActionArgs(ActionUpdate)
	if err != nil {
		t.Fatal(err)
	}
	expected := [][]string{{"pull"}, {"down"}, {"up", "-d"}}
	if !reflect.DeepEqual(update, expected) {
		t.Fatalf("update 命令序列错误: %+v", update)
	}
	if _, err := ActionArgs("unknown"); err == nil {
		t.Fatal("未知操作应报错")
	}
}
