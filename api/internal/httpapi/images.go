package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"judge/api/internal/images"
)

const maxContentImage = 512 << 10

// Decode the real format and re-encode to remove metadata and trailing payloads.
func cleanContentImage(data []byte) ([]byte, string, error) {
	invalid := errors.New("invalid image")
	if len(data) == 0 || len(data) > maxContentImage {
		return nil, "", invalid
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 2048 || cfg.Height > 2048 {
		return nil, "", invalid
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", invalid
	}
	var out bytes.Buffer
	if format == "png" {
		err = png.Encode(&out, img)
	} else {
		err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 85})
	}
	if err != nil || out.Len() > maxContentImage {
		return nil, "", invalid
	}
	return out.Bytes(), "image/" + format, nil
}

func (p imageHandler) publicImage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	p.contentImage(w, r.WithContext(ctx), "")
}

func (p imageHandler) contentImage(w http.ResponseWriter, r *http.Request, owner string) {
	w.Header().Set("Cache-Control", "no-store")
	if p.Store == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	ctx, id := r.Context(), r.PathValue("id")
	if id != "" && !problemID.MatchString(id) {
		authError(w, 404, "image_not_found")
		return
	}
	if r.Method == http.MethodGet && id != "" {
		data, media, err := p.Store.Get(ctx, id, owner)
		if errors.Is(err, pgx.ErrNoRows) {
			authError(w, 404, "image_not_found")
		} else if err != nil {
			authError(w, 503, "database_unavailable")
		} else {
			w.Header().Set("Content-Type", media)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			_, _ = w.Write(data)
		}
		return
	}
	if r.Method == http.MethodGet {
		offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
		if r.URL.Query().Get("offset") == "" {
			offset, err = 0, nil
		}
		if err != nil || offset < 0 || offset > 1000000 {
			authError(w, 400, "invalid_request")
			return
		}
		items, used, err := p.Store.List(ctx, owner, offset)
		if err != nil {
			authError(w, 503, "database_unavailable")
			return
		}
		more := len(items) > 50
		if more {
			items = items[:50]
		}

		writeAuthJSON(w, 200, map[string]any{"items": items, "usedBytes": used, "hasMore": more})
		return
	}
	if r.Method == http.MethodDelete {
		err := p.Store.Delete(ctx, owner, id)
		if errors.Is(err, pgx.ErrNoRows) {
			authError(w, 404, "image_not_found")
			return
		}
		if errors.Is(err, images.ErrInUse) {
			authError(w, 409, "image_in_use")
			return
		}
		if err != nil {
			authError(w, 503, "database_unavailable")
			return
		}

		w.WriteHeader(204)
		return
	}
	var input struct {
		Data string `json:"data"`
	}
	if !readJSONBody(w, r, &input, 720<<10) {
		return
	}
	raw, err := base64.StdEncoding.DecodeString(input.Data)
	if err != nil {
		authError(w, 400, "invalid_image")
		return
	}
	data, media, err := cleanContentImage(raw)
	if err != nil {
		authError(w, 400, "invalid_image")
		return
	}
	id, created, err := p.Store.Save(ctx, owner, newSubmissionID(), media, data)
	if err != nil {
		authError(w, 503, "database_unavailable")
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeAuthJSON(w, status, map[string]any{"id": id, "url": "/api/images/" + id, "size": len(data)})
}
