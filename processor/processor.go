package processor

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"path"
	"strings"

	"nextya-sync/models"
)

type cloudClient interface {
	ListFiles(ctx context.Context, folderPath string) ([]models.FileInfo, error)
	DownloadFile(ctx context.Context, path string) (io.ReadCloser, error)
	UploadFile(ctx context.Context, path string, content io.Reader, size int64) error
	CreateFolder(ctx context.Context, path string) error
	GetFileInfo(ctx context.Context, path string) (*models.FileInfo, error)
}

// Processor handles synchronization between cloud storage services
type Processor struct {
	yandexClient    cloudClient
	nextcloudClient cloudClient
}

// Dependencies configuration for creating a processor
type Dependencies struct {
	YandexClient    cloudClient
	NextcloudClient cloudClient
}

// Config holds configuration for the synchronization processor
type Config struct {
	YandexTargetPath   string
	NextcloudSyncPaths []string
}

// NewProcessor creates a new instance of synchronization processor
func NewProcessor(d *Dependencies) *Processor {
	return &Processor{
		yandexClient:    d.YandexClient,
		nextcloudClient: d.NextcloudClient,
	}
}

// Main main function for synchronizing files from Nextcloud to Yandex Disk
func (p *Processor) Main(ctx context.Context, cfg Config) error {
	log.Println("Starting synchronization from Nextcloud to Yandex Disk...")

	// Validate configuration
	if len(cfg.NextcloudSyncPaths) == 0 {
		return fmt.Errorf("no Nextcloud sync paths specified")
	}

	// Get file structure from Yandex Disk
	log.Println("Reading Yandex Disk file structure...")
	yandexTargetPath := cfg.YandexTargetPath
	yndxFs, err := p.getYandexFileSystem(ctx, yandexTargetPath)
	if err != nil {
		log.Printf("Yandex Disk target folder doesn't exist, will create it")
		// Create target folder chain if it doesn't exist
		if createErr := p.createFolderChain(ctx, yandexTargetPath); createErr != nil {
			return fmt.Errorf("failed to create target folder chain in Yandex Disk: %w", createErr)
		}
		// Create empty structure
		yndxFs = models.Folder{Path: yandexTargetPath}
	}

	// Synchronize each specified path
	syncStats := &SyncStats{}
	for _, syncPath := range cfg.NextcloudSyncPaths {
		log.Printf("Processing sync path: %s", syncPath)

		// Get file structure from Nextcloud for this specific path
		log.Printf("Reading Nextcloud file structure for path: %s", syncPath)
		ncFs, err := p.getNextcloudFileSystem(ctx, syncPath)
		if err != nil {
			log.Printf("Warning: failed to get Nextcloud file system for path %s: %v", syncPath, err)
			continue
		}

		// Determine target path in Yandex Disk
		// If syncing multiple paths, create subfolder for each path to avoid conflicts
		var targetPath string
		if len(cfg.NextcloudSyncPaths) == 1 {
			// If only one path, sync directly to target folder
			targetPath = yandexTargetPath
		} else {
			// If multiple paths, create subfolder based on the sync path name
			pathName := path.Base(syncPath)
			if pathName == "/" || pathName == "." {
				pathName = "root"
			}
			targetPath = yandexTargetPath + "/" + pathName
		}

		// Get or create corresponding Yandex folder structure
		var targetYandexFs models.Folder
		if targetPath == yandexTargetPath {
			targetYandexFs = yndxFs
		} else {
			// Try to get existing folder or create new one
			existingFs, err := p.getYandexFileSystem(ctx, targetPath)
			if err != nil {
				log.Printf("Target subfolder %s doesn't exist, will create it", targetPath)
				if createErr := p.yandexClient.CreateFolder(ctx, targetPath); createErr != nil {
					log.Printf("Warning: failed to create target subfolder %s: %v", targetPath, createErr)
					continue
				}
				targetYandexFs = models.Folder{Path: targetPath}
			} else {
				targetYandexFs = existingFs
			}
		}

		// Synchronize this specific path
		if err := p.syncFolders(ctx, ncFs, targetYandexFs, targetPath, syncStats); err != nil {
			log.Printf("Warning: synchronization failed for path %s: %v", syncPath, err)
			continue
		}
	}

	log.Printf("Synchronization completed! Files processed: %d, uploaded: %d, skipped: %d, errors: %d",
		syncStats.TotalFiles, syncStats.UploadedFiles, syncStats.SkippedFiles, syncStats.ErrorFiles)

	return nil
}

// SyncStats synchronization statistics
type SyncStats struct {
	TotalFiles    int
	UploadedFiles int
	SkippedFiles  int
	ErrorFiles    int
}

// createFolderChain creates a chain of folders recursively
func (p *Processor) createFolderChain(ctx context.Context, folderPath string) error {
	// Normalize path separators and remove leading/trailing slashes
	folderPath = strings.Trim(strings.ReplaceAll(folderPath, "\\", "/"), "/")

	if folderPath == "" {
		return nil
	}

	// Check if folder already exists
	_, err := p.yandexClient.GetFileInfo(ctx, folderPath)
	if err == nil {
		// Folder exists, nothing to do
		return nil
	}

	// Get parent folder path
	parentPath := path.Dir(folderPath)
	if parentPath != "." && parentPath != "/" && parentPath != folderPath {
		// Recursively create parent folder chain
		if createErr := p.createFolderChain(ctx, parentPath); createErr != nil {
			return fmt.Errorf("failed to create parent folder %s: %w", parentPath, createErr)
		}
	}

	// Create current folder
	log.Printf("Creating folder: %s", folderPath)
	if err := p.yandexClient.CreateFolder(ctx, folderPath); err != nil {
		return fmt.Errorf("failed to create folder %s: %w", folderPath, err)
	}

	return nil
}

// syncFolders synchronizes folders recursively
func (p *Processor) syncFolders(ctx context.Context, ncFolder, yandexFolder models.Folder, yandexBasePath string, stats *SyncStats) error {
	// Create Yandex Disk files map for quick lookup
	yandexFiles := make(map[string]models.File)
	for _, file := range yandexFolder.Files {
		fileName := path.Base(file.Path)
		yandexFiles[fileName] = file
	}

	// Create Yandex Disk folders map
	yandexFolders := make(map[string]models.Folder)
	for _, folder := range yandexFolder.Folders {
		folderName := path.Base(folder.Path)
		yandexFolders[folderName] = folder
	}

	// Synchronize files
	for _, ncFile := range ncFolder.Files {
		stats.TotalFiles++
		fileName := path.Base(ncFile.Path)

		// Decode filename for comparison and logging
		decodedFileName, err := url.QueryUnescape(fileName)
		if err != nil {
			log.Printf("Warning: failed to decode filename %s, using original: %v", fileName, err)
			decodedFileName = fileName
		}

		// Check if file needs synchronization (compare with decoded name)
		yandexFile, exists := yandexFiles[decodedFileName]
		needsSync := false

		if !exists {
			log.Printf("File %s doesn't exist in Yandex Disk, will upload", decodedFileName)
			needsSync = true
		} else {
			// Compare modification dates
			if ncFile.Modified.After(yandexFile.Modified) {
				log.Printf("File %s is newer in Nextcloud (%v vs %v), will update",
					decodedFileName, ncFile.Modified, yandexFile.Modified)
				needsSync = true
			} else {
				log.Printf("File %s is up to date, skipping", decodedFileName)
				stats.SkippedFiles++
			}
		}

		if needsSync {
			if err := p.syncFile(ctx, ncFile.Path, yandexBasePath, fileName); err != nil {
				log.Printf("Error syncing file %s: %v", decodedFileName, err)
				stats.ErrorFiles++
			} else {
				log.Printf("Successfully synced file %s", decodedFileName)
				stats.UploadedFiles++
			}
		}
	}

	// Recursively synchronize subfolders
	for _, ncSubFolder := range ncFolder.Folders {
		subFolderName := path.Base(ncSubFolder.Path)

		// Decode folder name
		decodedFolderName, err := url.QueryUnescape(subFolderName)
		if err != nil {
			log.Printf("Warning: failed to decode folder name %s, using original: %v", subFolderName, err)
			decodedFolderName = subFolderName
		}

		yandexSubFolderPath := yandexBasePath + "/" + decodedFolderName

		// Check if folder exists in Yandex Disk (compare with decoded name)
		yandexSubFolder, exists := yandexFolders[decodedFolderName]
		if !exists {
			log.Printf("Creating folder %s in Yandex Disk", yandexSubFolderPath)
			if err := p.createFolderChain(ctx, yandexSubFolderPath); err != nil {
				log.Printf("Error creating folder chain %s: %v", yandexSubFolderPath, err)
				continue
			}
			// Create empty structure for new folder
			yandexSubFolder = models.Folder{Path: yandexSubFolderPath}
		}

		// Recursively synchronize subfolder
		if err := p.syncFolders(ctx, ncSubFolder, yandexSubFolder, yandexSubFolderPath, stats); err != nil {
			log.Printf("Error syncing subfolder %s: %v", decodedFolderName, err)
		}
	}

	return nil
}

// syncFile synchronizes individual file
func (p *Processor) syncFile(ctx context.Context, ncFilePath, yandexBasePath, fileName string) error {
	// Decode filename from URL encoding
	decodedFileName, err := url.QueryUnescape(fileName)
	if err != nil {
		log.Printf("Warning: failed to decode filename %s, using original: %v", fileName, err)
		decodedFileName = fileName
	}

	// Download file from Nextcloud
	reader, err := p.nextcloudClient.DownloadFile(ctx, ncFilePath)
	if err != nil {
		return fmt.Errorf("failed to download file from Nextcloud: %w", err)
	}
	defer reader.Close()

	// Get file info for size
	ncFileInfo, err := p.nextcloudClient.GetFileInfo(ctx, ncFilePath)
	if err != nil {
		return fmt.Errorf("failed to get file info from Nextcloud: %w", err)
	}

	// Upload file to Yandex Disk with decoded name
	yandexFilePath := yandexBasePath + "/" + decodedFileName
	if err := p.yandexClient.UploadFile(ctx, yandexFilePath, reader, ncFileInfo.Size); err != nil {
		return fmt.Errorf("failed to upload file to Yandex Disk: %w", err)
	}

	return nil
}

func (p *Processor) getYandexFileSystem(ctx context.Context, rootPath string) (models.Folder, error) {
	files, err := p.yandexClient.ListFiles(ctx, rootPath)
	if err != nil {
		return models.Folder{}, err
	}

	folder := models.Folder{
		Path: rootPath,
	}

	for _, file := range files {
		if file.IsDir {
			subFolder, err := p.getYandexFileSystem(ctx, file.Path)
			if err != nil {
				return models.Folder{}, err
			}
			folder.Folders = append(folder.Folders, subFolder)
		} else {
			folder.Files = append(folder.Files, models.File{
				Path:     file.Path,
				Modified: file.ModTime,
			})
		}
	}
	return folder, nil
}

func (p *Processor) getNextcloudFileSystem(ctx context.Context, rootPath string) (models.Folder, error) {
	files, err := p.nextcloudClient.ListFiles(ctx, rootPath)
	if err != nil {
		return models.Folder{}, err
	}

	folder := models.Folder{
		Path: rootPath,
	}

	for _, file := range files {
		if file.IsDir {
			subFolder, err := p.getNextcloudFileSystem(ctx, file.Path)
			if err != nil {
				return models.Folder{}, err
			}
			folder.Folders = append(folder.Folders, subFolder)
		} else {
			folder.Files = append(folder.Files, models.File{
				Path:     file.Path,
				Modified: file.ModTime,
			})
		}
	}
	return folder, nil
}
