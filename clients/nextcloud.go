package clients

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"nextya-sync/models"

	"github.com/go-resty/resty/v2"
)

// RFC1123Time custom time type to handle RFC1123 format from WebDAV
type RFC1123Time struct {
	time.Time
}

// UnmarshalXML implements xml.Unmarshaler interface
func (t *RFC1123Time) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var v string
	if err := d.DecodeElement(&v, &start); err != nil {
		return err
	}

	if v == "" {
		t.Time = time.Time{}
		return nil
	}

	// Parse RFC1123 format: "Mon, 02 Jan 2006 15:04:05 MST"
	parsed, err := time.Parse(time.RFC1123, v)
	if err != nil {
		return err
	}

	t.Time = parsed
	return nil
}

// NextcloudClient client for working with Nextcloud API
type NextcloudClient struct {
	BaseURL  string
	Username string
	Password string
	client   *resty.Client
}

// NextcloudFileInfo structure for WebDAV response
type NextcloudFileInfo struct {
	XMLName xml.Name `xml:"response"`
	Href    string   `xml:"href"`
	Props   struct {
		DisplayName      string      `xml:"displayname"`
		GetLastModified  RFC1123Time `xml:"getlastmodified"`
		GetContentType   string      `xml:"getcontenttype"`
		GetContentLength int64       `xml:"getcontentlength"`
		GetETag          string      `xml:"getetag"`
		ResourceType     struct {
			Collection *struct{} `xml:"collection"`
		} `xml:"resourcetype"`
	} `xml:"propstat>prop"`
}

// MultiStatus structure for WebDAV PROPFIND response
type MultiStatus struct {
	XMLName   xml.Name            `xml:"multistatus"`
	Responses []NextcloudFileInfo `xml:"response"`
}

// NewNextcloudClient creates a new Nextcloud client
func NewNextcloudClient(baseURL, username, password string) *NextcloudClient {
	client := resty.New()
	client.SetDisableWarn(true)
	client.SetBasicAuth(username, password)
	client.SetHeader("OCS-APIRequest", "true")

	return &NextcloudClient{
		BaseURL:  strings.TrimSuffix(baseURL, "/"),
		Username: username,
		Password: password,
		client:   client,
	}
}

// Authenticate checks connection to Nextcloud
func (nc *NextcloudClient) Authenticate(ctx context.Context) error {
	resp, err := nc.client.R().
		SetContext(ctx).
		Get(nc.BaseURL + "/ocs/v1.php/cloud/capabilities")
	if err != nil {
		return fmt.Errorf("failed to connect to Nextcloud: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("authentication failed: status %d", resp.StatusCode())
	}

	return nil
}

// ListFiles gets list of files in folder
func (nc *NextcloudClient) ListFiles(ctx context.Context, folderPath string) ([]models.FileInfo, error) {
	webdavURL := nc.BaseURL + "/remote.php/dav/files/" + nc.Username + "/" + strings.TrimPrefix(folderPath, "/")

	resp, err := nc.client.R().
		SetContext(ctx).
		SetHeader("Depth", "1").
		SetBody(`<?xml version="1.0"?>
<d:propfind xmlns:d="DAV:">
  <d:prop>
    <d:displayname/>
    <d:getlastmodified/>
    <d:getcontenttype/>
    <d:getcontentlength/>
    <d:getetag/>
    <d:resourcetype/>
  </d:prop>
</d:propfind>`).
		Execute("PROPFIND", webdavURL)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	var multiStatus MultiStatus
	if err := xml.Unmarshal(resp.Body(), &multiStatus); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var files []models.FileInfo
	for i, response := range multiStatus.Responses {
		if i == 0 {
			continue // Skip first element (the folder itself)
		}

		isDir := response.Props.ResourceType.Collection != nil

		files = append(files, models.FileInfo{
			Name:        response.Props.DisplayName,
			Path:        strings.TrimPrefix(response.Href, "/remote.php/dav/files/"+nc.Username),
			Size:        response.Props.GetContentLength,
			IsDir:       isDir,
			ModTime:     response.Props.GetLastModified.Time,
			ETag:        strings.Trim(response.Props.GetETag, `"`),
			ContentType: response.Props.GetContentType,
		})
	}

	return files, nil
}

// UploadFile uploads file
func (nc *NextcloudClient) UploadFile(ctx context.Context, filePath string, content io.Reader, size int64) error {
	webdavURL := nc.BaseURL + "/remote.php/dav/files/" + nc.Username + "/" + strings.TrimPrefix(filePath, "/")

	resp, err := nc.client.R().
		SetContext(ctx).
		SetBody(content).
		SetHeader("Content-Length", strconv.FormatInt(size, 10)).
		Put(webdavURL)
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated && resp.StatusCode() != http.StatusNoContent {
		return fmt.Errorf("upload failed: status %d", resp.StatusCode())
	}

	return nil
}

// DownloadFile downloads file
func (nc *NextcloudClient) DownloadFile(ctx context.Context, filePath string) (io.ReadCloser, error) {
	webdavURL := nc.BaseURL + "/remote.php/dav/files/" + nc.Username + "/" + strings.TrimPrefix(filePath, "/")

	resp, err := nc.client.R().
		SetContext(ctx).
		SetDoNotParseResponse(true).
		Get(webdavURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		resp.RawBody().Close()
		return nil, fmt.Errorf("download failed: status %d", resp.StatusCode())
	}

	return resp.RawBody(), nil
}

// CreateFolder creates folder
func (nc *NextcloudClient) CreateFolder(ctx context.Context, folderPath string) error {
	webdavURL := nc.BaseURL + "/remote.php/dav/files/" + nc.Username + "/" + strings.TrimPrefix(folderPath, "/")

	resp, err := nc.client.R().
		SetContext(ctx).
		Execute("MKCOL", webdavURL)
	if err != nil {
		return fmt.Errorf("failed to create folder: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated {
		return fmt.Errorf("create folder failed: status %d", resp.StatusCode())
	}

	return nil
}

// GetFileInfo gets file information
func (nc *NextcloudClient) GetFileInfo(ctx context.Context, filePath string) (*models.FileInfo, error) {
	files, err := nc.ListFiles(ctx, path.Dir(filePath))
	if err != nil {
		return nil, err
	}

	fileName := path.Base(filePath)
	for _, file := range files {
		if path.Base(file.Path) == fileName {
			return &file, nil
		}
	}

	return nil, fmt.Errorf("file not found: %s", filePath)
}
