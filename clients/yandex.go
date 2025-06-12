package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"nextya-sync/models"

	"github.com/go-resty/resty/v2"
)

// YandexDiskClient client for working with Yandex Disk API
type YandexDiskClient struct {
	Token  string
	client *resty.Client
}

// YandexDiskResource structure for file/folder in Yandex Disk
type YandexDiskResource struct {
	Type     string    `json:"type"`
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	MimeType string    `json:"mime_type"`
	File     string    `json:"file,omitempty"`
}

// YandexDiskResourceList structure for file list
type YandexDiskResourceList struct {
	Items []YandexDiskResource `json:"items"`
	Total int                  `json:"total"`
}

// YandexDiskLink structure for upload/download links
type YandexDiskLink struct {
	Href      string `json:"href"`
	Method    string `json:"method"`
	Templated bool   `json:"templated"`
}

// NewYandexDiskClient creates a new Yandex Disk client with token
func NewYandexDiskClient(token string) *YandexDiskClient {
	if token == "" {
		return nil
	}

	client := resty.New()
	client.SetHeader("Authorization", "OAuth "+token)
	client.SetHeader("Content-Type", "application/json")

	return &YandexDiskClient{
		Token:  token,
		client: client,
	}
}

// Authenticate checks token validity
func (yd *YandexDiskClient) Authenticate(ctx context.Context) error {
	resp, err := yd.client.R().
		SetContext(ctx).
		Get("https://cloud-api.yandex.net/v1/disk/")
	if err != nil {
		return fmt.Errorf("failed to connect to Yandex Disk: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("authentication failed: status %d", resp.StatusCode())
	}

	return nil
}

// ListFiles gets list of files in folder
func (yd *YandexDiskClient) ListFiles(ctx context.Context, folderPath string) ([]models.FileInfo, error) {
	if folderPath == "" {
		folderPath = "/"
	}

	resp, err := yd.client.R().
		SetContext(ctx).
		SetQueryParam("path", folderPath).
		SetQueryParam("limit", "1000").
		Get("https://cloud-api.yandex.net/v1/disk/resources")
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("list files failed: status %d", resp.StatusCode())
	}

	var folderInfo struct {
		Embedded YandexDiskResourceList `json:"_embedded"`
	}

	if err := json.Unmarshal(resp.Body(), &folderInfo); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var files []models.FileInfo
	for _, item := range folderInfo.Embedded.Items {
		files = append(files, models.FileInfo{
			Name:        item.Name,
			Path:        item.Path,
			Size:        item.Size,
			IsDir:       item.Type == "dir",
			ModTime:     item.Modified,
			ContentType: item.MimeType,
			DownloadURL: item.File,
		})
	}

	return files, nil
}

// UploadFile uploads file
func (yd *YandexDiskClient) UploadFile(ctx context.Context, filePath string, content io.Reader, size int64) error {
	// Get upload URL
	uploadURL, err := yd.getUploadURL(ctx, filePath)
	if err != nil {
		return fmt.Errorf("failed to get upload URL: %w", err)
	}

	// Upload file using the obtained URL
	resp, err := yd.client.R().
		SetContext(ctx).
		SetBody(content).
		SetHeader("Content-Length", strconv.FormatInt(size, 10)).
		Put(uploadURL)
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated {
		return fmt.Errorf("upload failed: status %d", resp.StatusCode())
	}

	return nil
}

// getUploadURL gets URL for file upload
func (yd *YandexDiskClient) getUploadURL(ctx context.Context, filePath string) (string, error) {
	resp, err := yd.client.R().
		SetContext(ctx).
		SetQueryParam("path", filePath).
		SetQueryParam("overwrite", "true").
		Get("https://cloud-api.yandex.net/v1/disk/resources/upload")
	if err != nil {
		return "", err
	}

	if resp.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("get upload URL failed: status %d", resp.StatusCode())
	}

	var link YandexDiskLink
	if err := json.Unmarshal(resp.Body(), &link); err != nil {
		return "", fmt.Errorf("failed to parse upload URL response: %w", err)
	}

	return link.Href, nil
}

// DownloadFile downloads file
func (yd *YandexDiskClient) DownloadFile(ctx context.Context, filePath string) (io.ReadCloser, error) {
	// Get download URL
	downloadURL, err := yd.getDownloadURL(ctx, filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get download URL: %w", err)
	}

	// Download file using the obtained URL
	resp, err := yd.client.R().
		SetContext(ctx).
		SetDoNotParseResponse(true).
		Get(downloadURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		resp.RawBody().Close()
		return nil, fmt.Errorf("download failed: status %d", resp.StatusCode())
	}

	return resp.RawBody(), nil
}

// getDownloadURL gets URL for file download
func (yd *YandexDiskClient) getDownloadURL(ctx context.Context, filePath string) (string, error) {
	resp, err := yd.client.R().
		SetContext(ctx).
		SetQueryParam("path", filePath).
		Get("https://cloud-api.yandex.net/v1/disk/resources/download")
	if err != nil {
		return "", err
	}

	if resp.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("get download URL failed: status %d", resp.StatusCode())
	}

	var link YandexDiskLink
	if err := json.Unmarshal(resp.Body(), &link); err != nil {
		return "", fmt.Errorf("failed to parse download URL response: %w", err)
	}

	return link.Href, nil
}

// CreateFolder creates folder
func (yd *YandexDiskClient) CreateFolder(ctx context.Context, folderPath string) error {
	resp, err := yd.client.R().
		SetContext(ctx).
		SetQueryParam("path", folderPath).
		Put("https://cloud-api.yandex.net/v1/disk/resources")
	if err != nil {
		return fmt.Errorf("failed to create folder: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated {
		return fmt.Errorf("create folder failed: status %d", resp.StatusCode())
	}

	return nil
}

// GetFileInfo gets file information
func (yd *YandexDiskClient) GetFileInfo(ctx context.Context, filePath string) (*models.FileInfo, error) {
	resp, err := yd.client.R().
		SetContext(ctx).
		SetQueryParam("path", filePath).
		Get("https://cloud-api.yandex.net/v1/disk/resources")
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("get file info failed: status %d", resp.StatusCode())
	}

	var resource YandexDiskResource
	if err := json.Unmarshal(resp.Body(), &resource); err != nil {
		return nil, fmt.Errorf("failed to parse file info response: %w", err)
	}

	return &models.FileInfo{
		Name:        resource.Name,
		Path:        resource.Path,
		Size:        resource.Size,
		IsDir:       resource.Type == "dir",
		ModTime:     resource.Modified,
		ContentType: resource.MimeType,
		DownloadURL: resource.File,
	}, nil
}
