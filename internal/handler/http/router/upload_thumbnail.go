package router

import (
	"image"
	"image/jpeg"
	_ "image/gif"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"golang.org/x/image/draw"
)

const uploadsDir = "./storage/uploads"

func isUnderUploadsRoot(rootAbs, candidateAbs string) bool {
	rel, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func fitInsideMax(src image.Image, max int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return src
	}
	if w <= max && h <= max {
		return src
	}
	var nw, nh int
	if w >= h {
		nw = max
		nh = int(float64(h)*float64(max)/float64(w) + 0.5)
		if nh < 1 {
			nh = 1
		}
	} else {
		nh = max
		nw = int(float64(w)*float64(max)/float64(h) + 0.5)
		if nw < 1 {
			nw = 1
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}

// serveUploadThumbnail serves a downscaled JPEG for files under storage/uploads.
// Query: path=relative/path/under/uploads&w=160 (w clamped 32..512, default 160).
// Public (same as /uploads/* static); use opaque filenames in production.
func serveUploadThumbnail(log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rel := strings.TrimSpace(r.URL.Query().Get("path"))
		rel = filepath.ToSlash(rel)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" || strings.Contains(rel, "..") {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		maxW := 160
		if ws := strings.TrimSpace(r.URL.Query().Get("w")); ws != "" {
			if n, err := strconv.Atoi(ws); err == nil {
				switch {
				case n < 32:
					maxW = 32
				case n > 512:
					maxW = 512
				default:
					maxW = n
				}
			}
		}

		rootAbs, err := filepath.Abs(uploadsDir)
		if err != nil {
			log.Error("upload thumb root abs", zap.Error(err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		full := filepath.Join(uploadsDir, filepath.FromSlash(rel))
		fullAbs, err := filepath.Abs(full)
		if err != nil || !isUnderUploadsRoot(rootAbs, fullAbs) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		f, err := os.Open(fullAbs)
		if err != nil {
			if os.IsNotExist(err) {
				http.NotFound(w, r)
				return
			}
			log.Error("upload thumb open", zap.Error(err), zap.String("path", fullAbs))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer f.Close()

		img, _, err := image.Decode(f)
		if err != nil {
			log.Debug("upload thumb decode", zap.Error(err), zap.String("path", fullAbs))
			http.Error(w, "unsupported or corrupt image", http.StatusUnsupportedMediaType)
			return
		}

		out := fitInsideMax(img, maxW)
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=604800")
		if err := jpeg.Encode(w, out, &jpeg.Options{Quality: 82}); err != nil {
			log.Error("upload thumb encode", zap.Error(err))
		}
	}
}
