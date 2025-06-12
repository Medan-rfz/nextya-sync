package models

import "time"

// FileInfo represents file information
type FileInfo struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	IsDir       bool      `json:"is_dir"`
	ModTime     time.Time `json:"mod_time"`
	ETag        string    `json:"etag,omitempty"`
	ContentType string    `json:"content_type,omitempty"`
	DownloadURL string    `json:"download_url,omitempty"`
}
