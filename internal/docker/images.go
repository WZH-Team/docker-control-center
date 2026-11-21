package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ListImages 列出全部镜像。
func (c *Client) ListImages(ctx context.Context) ([]ImageSummary, error) {
	var out []ImageSummary
	if err := c.doJSON(ctx, http.MethodGet, "/images/json", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// InspectImage 获取镜像详情（ref 可为镜像 ID 或 名称:标签）。
func (c *Client) InspectImage(ctx context.Context, ref string) (*ImageInspect, error) {
	var out ImageInspect
	if err := c.doJSON(ctx, http.MethodGet, "/images/"+url.PathEscape(ref)+"/json", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RemoveImage 删除镜像，返回删除结果（untagged/deleted 明细）。
func (c *Client) RemoveImage(ctx context.Context, ref string, force bool) ([]ImageDeleteResponseItem, error) {
	q := url.Values{}
	if force {
		q.Set("force", "true")
	}
	var out []ImageDeleteResponseItem
	if err := c.doJSON(ctx, http.MethodDelete, "/images/"+url.PathEscape(ref), q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// PullImage 拉取镜像，将拉取进度以可读文本写入 out。
// 镜像引用支持 name、name:tag、name@digest 形式。
func (c *Client) PullImage(ctx context.Context, ref string, out io.Writer) error {
	name, tag, err := ParseImageRef(ref)
	if err != nil {
		return err
	}
	q := url.Values{}
	q.Set("fromImage", name)
	q.Set("tag", tag)

	headers := map[string]string{}
	if auth := registryAuthHeader(name); auth != "" {
		headers["X-Registry-Auth"] = auth
	}

	resp, err := c.do(ctx, http.MethodPost, "/images/create", q, nil, headers)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return decodeError(resp)
	}

	dec := json.NewDecoder(resp.Body)
	for {
		var line PullProgress
		if err := dec.Decode(&line); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				break
			}
			// 连接中断等异常结束。
			if ctx.Err() != nil {
				return fmt.Errorf("拉取已取消: %w", ctx.Err())
			}
			return fmt.Errorf("读取拉取进度失败: %w", err)
		}
		if line.Error != "" {
			msg := line.Error
			if line.ErrorDetail.Message != "" {
				msg = line.ErrorDetail.Message
			}
			return fmt.Errorf("拉取镜像失败: %s", msg)
		}
		// 进度条（带 progress 字段）逐帧刷新，不写入日志；只输出里程碑信息。
		if line.Progress != "" {
			continue
		}
		msg := strings.TrimSpace(line.Status)
		if line.ID != "" {
			msg = fmt.Sprintf("[%s] %s", line.ID, msg)
		}
		if msg != "" && out != nil {
			fmt.Fprintln(out, msg)
		}
	}
	return nil
}

// ImageUsage 返回正在使用指定镜像 ID 的容器列表（含已停止容器）。
func (c *Client) ImageUsage(ctx context.Context, imageID string) ([]ContainerSummary, error) {
	all, err := c.ListContainers(ctx, true)
	if err != nil {
		return nil, err
	}
	target := TrimImageID(imageID)
	var used []ContainerSummary
	for _, ctr := range all {
		if TrimImageID(ctr.ImageID) == target {
			used = append(used, ctr)
			continue
		}
		// 兜底：按镜像引用名比较（镜像 ID 缺失或不一致时）。
		if imageID != "" && ctr.Image == imageID {
			used = append(used, ctr)
		}
	}
	return used, nil
}
