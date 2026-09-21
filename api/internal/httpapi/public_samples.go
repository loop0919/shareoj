package httpapi

import (
	"context"
	"net/http"
	"time"

	"judge/api/internal/problems"
	"judge/api/internal/testfiles"
)

type publicSample struct {
	Name       string              `json:"name"`
	Input      string              `json:"input"`
	Output     string              `json:"output"`
	InputFile  *testfiles.Download `json:"inputFile,omitempty"`
	OutputFile *testfiles.Download `json:"outputFile,omitempty"`
}

func (p problemHandler) publicSamples(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	id := r.PathValue("id")
	if !problemID.MatchString(id) {
		authError(w, 404, "not_found")
		return
	}
	store, ok := p.Store.(interface {
		PublicSamples(context.Context, string) ([]problems.TestCase, error)
	})
	if !ok {
		authError(w, 503, "database_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	cases, err := store.PublicSamples(ctx, id)
	if err != nil {
		problemError(w, err)
		return
	}
	items := make([]publicSample, 0, len(cases))
	for _, c := range cases {
		item := publicSample{Name: c.Name, Input: c.Input, Output: c.Output}
		for _, side := range []struct {
			ref *problems.TestFile
			dst **testfiles.Download
		}{{c.InputFile, &item.InputFile}, {c.OutputFile, &item.OutputFile}} {
			if side.ref == nil {
				continue
			}
			files, ok := p.Files.(interface {
				PublicSampleDownload(context.Context, string, string) (testfiles.Download, error)
			})
			if !ok {
				authError(w, 503, "test_file_storage_unavailable")
				return
			}
			download, err := files.PublicSampleDownload(ctx, id, side.ref.ID)
			if err != nil {
				authError(w, 503, "test_file_storage_unavailable")
				return
			}
			*side.dst = &download
		}
		items = append(items, item)
	}
	writeAuthJSON(w, 200, itemsResponse[publicSample]{Items: items})
}
