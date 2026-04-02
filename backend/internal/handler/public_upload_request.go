package handler

import (
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"

	"openshare/backend/internal/service"
)

var (
	errUploadFormParse        = errors.New("failed to parse upload form")
	errUploadBodyTooLarge     = errors.New("upload request body too large")
	errUploadManifestInvalid  = errors.New("invalid upload manifest")
	errUploadManifestMismatch = errors.New("upload files do not match manifest")
	errUploadFileRequired     = errors.New("file is required")
	errUploadFileRead         = errors.New("failed to read uploaded file")
	errUploadTotalTooLarge    = errors.New("upload total size too large")
)

const (
	minUploadRequestOverheadBytes = 1 << 20
	maxUploadRequestOverheadBytes = 32 << 20
)

type uploadManifestEntry struct {
	RelativePath string `json:"relative_path"`
}

type parsedPublicUploadRequest struct {
	input   service.PublicUploadInput
	closers []io.Closer
}

func (r *parsedPublicUploadRequest) Close() {
	for _, closer := range r.closers {
		_ = closer.Close()
	}
}

func (h *PublicUploadHandler) parseSubmissionRequest(ctx *gin.Context) (*parsedPublicUploadRequest, error) {
	maxUploadTotalBytes := h.service.MaxUploadTotalBytes(ctx.Request.Context())
	if maxUploadTotalBytes > 0 {
		ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, requestBodyLimit(maxUploadTotalBytes))
	}

	form, err := ctx.MultipartForm()
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, errUploadBodyTooLarge
		}
		return nil, errUploadFormParse
	}

	fileHeaders, manifest, err := extractUploadFiles(form, ctx.PostForm("manifest"))
	if err != nil {
		return nil, err
	}
	if maxUploadTotalBytes > 0 && totalUploadBytes(fileHeaders) > maxUploadTotalBytes {
		return nil, errUploadTotalTooLarge
	}

	files := make([]service.PublicUploadFileInput, 0, len(fileHeaders))
	closers := make([]io.Closer, 0, len(fileHeaders))
	for index, fileHeader := range fileHeaders {
		file, openErr := fileHeader.Open()
		if openErr != nil {
			closeClosers(closers)
			return nil, errUploadFileRead
		}
		closers = append(closers, file)
		files = append(files, service.PublicUploadFileInput{
			Name:         fileHeader.Filename,
			RelativePath: manifest[index].RelativePath,
			File:         file,
		})
	}

	return &parsedPublicUploadRequest{
		input: service.PublicUploadInput{
			Description: ctx.PostForm("description"),
			ReceiptCode: readPublicReceiptCode(ctx),
			FolderID:    ctx.PostForm("folder_id"),
			UploaderIP:  ctx.ClientIP(),
			Files:       files,
		},
		closers: closers,
	}, nil
}

func extractUploadFiles(form *multipart.Form, manifestRaw string) ([]*multipart.FileHeader, []uploadManifestEntry, error) {
	if manifestRaw == "" {
		fileHeaders := form.File["file"]
		if len(fileHeaders) == 0 {
			return nil, nil, errUploadFileRequired
		}

		manifest := make([]uploadManifestEntry, 0, len(fileHeaders))
		for _, fileHeader := range fileHeaders {
			manifest = append(manifest, uploadManifestEntry{RelativePath: fileHeader.Filename})
		}
		return fileHeaders, manifest, nil
	}

	var manifest []uploadManifestEntry
	if err := json.Unmarshal([]byte(manifestRaw), &manifest); err != nil {
		return nil, nil, errUploadManifestInvalid
	}

	fileHeaders := form.File["files"]
	if len(fileHeaders) == 0 || len(fileHeaders) != len(manifest) {
		return nil, nil, errUploadManifestMismatch
	}
	return fileHeaders, manifest, nil
}

func requestBodyLimit(maxUploadTotalBytes int64) int64 {
	if maxUploadTotalBytes <= 0 {
		return 0
	}

	overhead := maxUploadTotalBytes / 100
	if overhead < minUploadRequestOverheadBytes {
		overhead = minUploadRequestOverheadBytes
	}
	if overhead > maxUploadRequestOverheadBytes {
		overhead = maxUploadRequestOverheadBytes
	}
	return maxUploadTotalBytes + overhead
}

func totalUploadBytes(fileHeaders []*multipart.FileHeader) int64 {
	var total int64
	for _, fileHeader := range fileHeaders {
		if fileHeader == nil || fileHeader.Size <= 0 {
			continue
		}
		total += fileHeader.Size
	}
	return total
}

func closeClosers(closers []io.Closer) {
	for _, closer := range closers {
		_ = closer.Close()
	}
}
