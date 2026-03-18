package webui

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

func Register(engine *gin.Engine) {
	dist, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("webui: load embedded dist: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(dist))

	engine.NoRoute(func(ctx *gin.Context) {
		requestPath := ctx.Request.URL.Path
		if strings.HasPrefix(requestPath, "/api/") || requestPath == "/api" {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		target := strings.TrimPrefix(path.Clean("/"+requestPath), "/")
		if target == "" || target == "." {
			target = "index.html"
		}

		if hasFile(dist, target) {
			serveFile(ctx, fileServer, "/"+target)
			return
		}

		serveFile(ctx, fileServer, "/index.html")
	})
}

func hasFile(fsys fs.FS, name string) bool {
	file, err := fsys.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return false
	}

	return !info.IsDir()
}

func serveFile(ctx *gin.Context, handler http.Handler, name string) {
	originalPath := ctx.Request.URL.Path
	ctx.Request.URL.Path = name
	handler.ServeHTTP(ctx.Writer, ctx.Request)
	ctx.Request.URL.Path = originalPath
}
