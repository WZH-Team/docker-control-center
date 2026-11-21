package docker

import (
	"context"
	"net/http"
	"net/url"
)

// ListVolumes 列出全部存储卷。
func (c *Client) ListVolumes(ctx context.Context) ([]*Volume, error) {
	var out VolumeListResponse
	if err := c.doJSON(ctx, http.MethodGet, "/volumes", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Volumes, nil
}

// InspectVolume 获取存储卷详情。
func (c *Client) InspectVolume(ctx context.Context, name string) (*Volume, error) {
	var out Volume
	if err := c.doJSON(ctx, http.MethodGet, "/volumes/"+url.PathEscape(name), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RemoveVolume 删除存储卷。
func (c *Client) RemoveVolume(ctx context.Context, name string, force bool) error {
	q := url.Values{}
	if force {
		q.Set("force", "true")
	}
	return c.doJSON(ctx, http.MethodDelete, "/volumes/"+url.PathEscape(name), q, nil, nil)
}
