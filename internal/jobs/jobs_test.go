package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func TestJobLifecycle(t *testing.T) {
	m := NewManager(nil)

	j := m.Start("test", "测试任务", func(ctx context.Context, w io.Writer) error {
		fmt.Fprint(w, "hello ")
		fmt.Fprint(w, "world")
		return nil
	})

	// 等待任务结束。
	deadline := time.Now().Add(3 * time.Second)
	for j.Status() == StatusRunning && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	info := j.Snapshot()
	if info.Status != StatusSuccess {
		t.Fatalf("任务应成功，实际 %s (err=%v)", info.Status, info.Error)
	}
	if info.Output != "hello world" {
		t.Fatalf("输出错误: %q", info.Output)
	}
	if info.StartedAt == nil || info.FinishedAt == nil {
		t.Fatal("缺少时间戳")
	}

	// Recent 可查询。
	recent := m.Recent(10)
	if len(recent) != 1 || recent[0].ID != j.ID {
		t.Fatalf("Recent 结果错误: %+v", recent)
	}
	if got, ok := m.Get(j.ID); !ok || got.ID != j.ID {
		t.Fatal("Get 失败")
	}
}

func TestJobFailureAndCancel(t *testing.T) {
	m := NewManager(nil)

	fail := m.Start("test", "失败任务", func(ctx context.Context, w io.Writer) error {
		return errors.New("boom")
	})
	cancelable := m.Start("test", "取消任务", func(ctx context.Context, w io.Writer) error {
		<-ctx.Done()
		return ctx.Err()
	})
	cancelable.Cancel()

	deadline := time.Now().Add(3 * time.Second)
	for (fail.Status() == StatusRunning || cancelable.Status() == StatusRunning) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if fail.Status() != StatusFailed || fail.Snapshot().Error != "boom" {
		t.Fatalf("失败任务状态错误: %+v", fail.Snapshot())
	}
	if cancelable.Status() != StatusCanceled {
		t.Fatalf("任务应为 canceled，实际 %s", cancelable.Status())
	}
}

func TestOutputCap(t *testing.T) {
	m := NewManager(nil)
	j := m.Start("test", "大输出", func(ctx context.Context, w io.Writer) error {
		line := strings.Repeat("x", 1024) + "\n"
		for i := 0; i < maxOutputBytes/1024+10; i++ {
			if _, err := io.WriteString(w, line); err != nil {
				return err
			}
		}
		return nil
	})

	deadline := time.Now().Add(5 * time.Second)
	for j.Status() == StatusRunning && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	info := j.Snapshot()
	if len(info.Output) > maxOutputBytes {
		t.Fatalf("输出应被截断，实际 %d 字节", len(info.Output))
	}
	if !info.OutputFull {
		t.Fatal("应标记输出被截断")
	}
}
