package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"mem/webservice/internal/db"
)

const (
	MaxSingleFileSize = 10 * 1024 * 1024   // 10MB
	MaxTotalUserQuota = 1024 * 1024 * 1024 // 1GB
)

type FileListResponse struct {
	Files     []db.FileRecord `json:"files"`
	TotalSize int64           `json:"total_size"`
	MaxSize   int64           `json:"max_size"`
}

// EnsureUserStorageQuota removes oldest files until current total + newFileSize <= MaxTotalUserQuota
func EnsureUserStorageQuota(database *db.DB, uploadDir string, userID int64, newFileSize int64) error {
	currentTotal, err := database.GetUserTotalFileSize(userID)
	if err != nil {
		return err
	}
	if currentTotal+newFileSize <= MaxTotalUserQuota {
		return nil
	}

	oldestFiles, err := database.GetOldestFiles(userID)
	if err != nil {
		return err
	}

	for _, oldFile := range oldestFiles {
		if currentTotal+newFileSize <= MaxTotalUserQuota {
			break
		}
		// Delete record and disk file
		_, delErr := database.DeleteFile(userID, oldFile.ID)
		if delErr == nil {
			_ = os.Remove(filepath.Join(uploadDir, oldFile.StoredName))
			currentTotal -= oldFile.FileSize
		}
	}
	return nil
}

func serveFileDownload(w http.ResponseWriter, r *http.Request, uploadDir string, fileRec *db.FileRecord) {
	filePath := filepath.Join(uploadDir, fileRec.StoredName)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "File not found on disk", http.StatusNotFound)
		return
	}

	encodedName := url.PathEscape(fileRec.FileName)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q; filename*=UTF-8''%s", fileRec.FileName, encodedName))
	if fileRec.ContentType != "" {
		w.Header().Set("Content-Type", fileRec.ContentType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Content-Length", strconv.FormatInt(fileRec.FileSize, 10))

	http.ServeFile(w, r, filePath)
}
