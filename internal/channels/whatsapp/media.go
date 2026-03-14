package whatsapp

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/channels/media"
)

const (
	// maxMediaBytes is the max download size for WhatsApp media (20 MB).
	maxMediaBytes int64 = 20 * 1024 * 1024

	// downloadTimeout is the HTTP timeout for downloading media from URLs.
	downloadTimeout = 30 * time.Second
)

// resolveMedia processes raw media entries from the bridge.
// Each entry can be a URL (http/https) or a local file path.
// Returns MediaInfo list with local file paths and detected MIME types.
func (c *Channel) resolveMedia(rawMedia []any) []media.MediaInfo {
	var results []media.MediaInfo

	for _, m := range rawMedia {
		switch v := m.(type) {
		case string:
			info := c.resolveMediaEntry(v, "")
			if info != nil {
				results = append(results, *info)
			}

		case map[string]any:
			// Bridge may send structured media: {"url":"...","filename":"...","mimetype":"..."}
			url, _ := v["url"].(string)
			path, _ := v["path"].(string)
			fileName, _ := v["filename"].(string)
			mimeType, _ := v["mimetype"].(string)

			target := url
			if target == "" {
				target = path
			}
			if target == "" {
				continue
			}

			info := c.resolveMediaEntry(target, fileName)
			if info != nil {
				if mimeType != "" {
					info.ContentType = mimeType
					info.Type = media.MediaKindFromMime(mimeType)
				}
				if fileName != "" {
					info.FileName = fileName
				}
				results = append(results, *info)
			}
		}
	}

	return results
}

// resolveMediaEntry handles a single media entry (URL or local path).
func (c *Channel) resolveMediaEntry(entry, fileName string) *media.MediaInfo {
	if strings.HasPrefix(entry, "http://") || strings.HasPrefix(entry, "https://") {
		return c.downloadMediaURL(entry, fileName)
	}
	return c.resolveLocalFile(entry, fileName)
}

// downloadMediaURL downloads media from a URL and saves to a temp file.
func (c *Channel) downloadMediaURL(url, fileName string) *media.MediaInfo {
	client := &http.Client{Timeout: downloadTimeout}

	resp, err := client.Get(url)
	if err != nil {
		slog.Warn("whatsapp media download failed", "url", url, "error", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("whatsapp media download non-200", "url", url, "status", resp.StatusCode)
		return nil
	}

	// Detect extension from Content-Type or URL
	ext := extensionFromContentType(resp.Header.Get("Content-Type"))
	if ext == "" && fileName != "" {
		ext = filepath.Ext(fileName)
	}
	if ext == "" {
		ext = extensionFromURL(url)
	}

	f, err := os.CreateTemp("", "goclaw_wa_*"+ext)
	if err != nil {
		slog.Warn("whatsapp media temp file failed", "error", err)
		return nil
	}
	defer f.Close()

	n, err := io.Copy(f, io.LimitReader(resp.Body, maxMediaBytes))
	if err != nil {
		os.Remove(f.Name())
		slog.Warn("whatsapp media write failed", "error", err)
		return nil
	}
	if n == 0 {
		os.Remove(f.Name())
		slog.Warn("whatsapp media empty response", "url", url)
		return nil
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = media.DetectMIMEType(f.Name())
	}
	kind := media.MediaKindFromMime(ct)

	if fileName == "" {
		fileName = filepath.Base(f.Name())
	}

	slog.Debug("whatsapp media downloaded", "path", f.Name(), "size", n, "type", kind)

	return &media.MediaInfo{
		Type:        kind,
		FilePath:    f.Name(),
		ContentType: ct,
		FileName:    fileName,
		FileSize:    n,
	}
}

// resolveLocalFile validates a local file path from the bridge.
func (c *Channel) resolveLocalFile(path, fileName string) *media.MediaInfo {
	info, err := os.Stat(path)
	if err != nil {
		slog.Warn("whatsapp media local file not found", "path", path, "error", err)
		return nil
	}

	if info.IsDir() {
		return nil
	}

	if info.Size() > maxMediaBytes {
		slog.Warn("whatsapp media file too large", "path", path, "size", info.Size())
		return nil
	}

	ct := media.DetectMIMEType(path)
	kind := media.MediaKindFromMime(ct)

	if fileName == "" {
		fileName = filepath.Base(path)
	}

	slog.Debug("whatsapp media local file resolved", "path", path, "size", info.Size(), "type", kind)

	return &media.MediaInfo{
		Type:        kind,
		FilePath:    path,
		ContentType: ct,
		FileName:    fileName,
		FileSize:    info.Size(),
	}
}

// extensionFromContentType maps common Content-Type headers to file extensions.
func extensionFromContentType(ct string) string {
	ct = strings.ToLower(ct)
	switch {
	case strings.Contains(ct, "image/jpeg"):
		return ".jpg"
	case strings.Contains(ct, "image/png"):
		return ".png"
	case strings.Contains(ct, "image/gif"):
		return ".gif"
	case strings.Contains(ct, "image/webp"):
		return ".webp"
	case strings.Contains(ct, "video/mp4"):
		return ".mp4"
	case strings.Contains(ct, "audio/ogg"):
		return ".ogg"
	case strings.Contains(ct, "audio/mpeg"):
		return ".mp3"
	case strings.Contains(ct, "application/pdf"):
		return ".pdf"
	case strings.Contains(ct, "application/vnd.openxmlformats"):
		return ".docx"
	default:
		return ""
	}
}

// extensionFromURL extracts the file extension from a URL path.
func extensionFromURL(url string) string {
	// Strip query string
	if idx := strings.IndexByte(url, '?'); idx > 0 {
		url = url[:idx]
	}
	ext := filepath.Ext(url)
	if len(ext) > 6 { // sanity check
		return ""
	}
	return ext
}

// mediaInfoToPaths converts MediaInfo slice to string paths for HandleMessage.
func mediaInfoToPaths(infos []media.MediaInfo) []string {
	paths := make([]string, 0, len(infos))
	for _, info := range infos {
		paths = append(paths, info.FilePath)
	}
	return paths
}

// mediaInfoToLogAttrs returns a summary string for logging.
func mediaInfoToLogAttrs(infos []media.MediaInfo) string {
	if len(infos) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(infos))
	for _, info := range infos {
		parts = append(parts, fmt.Sprintf("%s(%s)", info.Type, info.FileName))
	}
	return strings.Join(parts, ", ")
}
