package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"judge/api/internal/folders"
)

type folderHandler struct{ Store *folders.Store }

// folderList is the my page's folders and what they hold.
type folderList struct {
	Folders []folders.Folder `json:"folders"`
	Items   []folders.Item   `json:"items"`
}

// sentFolder is a folder id of a body, remembering whether the body named it: null names no folder, which is the top
// for a parent and unsorted for a placement.
type sentFolder struct {
	sent bool
	id   *string
}

func (p *sentFolder) UnmarshalJSON(data []byte) error {
	p.sent = true
	return json.Unmarshal(data, &p.id)
}

type folderBody struct {
	Name     *string    `json:"name"`
	ParentID sentFolder `json:"parentId"`
}

type placementBody struct {
	FolderID sentFolder    `json:"folderId"`
	Items    []folders.Ref `json:"items"`
}

func (h folderHandler) list(w http.ResponseWriter, r *http.Request, owner string) {
	if h.Store == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	list, err := h.Store.Folders(r.Context(), owner)
	if err != nil {
		folderError(w, err)
		return
	}
	items, err := h.Store.Items(r.Context(), owner)
	if err != nil {
		folderError(w, err)
		return
	}
	writeAuthJSON(w, 200, folderList{Folders: list, Items: items})
}

// create makes a folder; without a parentId it is at the top.
func (h folderHandler) create(w http.ResponseWriter, r *http.Request, owner string) {
	if h.Store == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	var in folderBody
	if !contentJSON(w, r, &in) {
		return
	}
	if in.Name == nil {
		authError(w, 400, "invalid_request")
		return
	}
	folder, err := h.Store.Create(r.Context(), owner, *in.Name, in.ParentID.id)
	if err != nil {
		folderError(w, err)
		return
	}
	writeAuthJSON(w, 201, folder)
}

// update renames and moves a folder. It needs both fields, so that a client that leaves out the parent cannot move
// the folder to the top.
func (h folderHandler) update(w http.ResponseWriter, r *http.Request, owner string) {
	if h.Store == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	var in folderBody
	if !contentJSON(w, r, &in) {
		return
	}
	if in.Name == nil || !in.ParentID.sent {
		authError(w, 400, "invalid_request")
		return
	}
	folder, err := h.Store.Update(r.Context(), owner, r.PathValue("id"), *in.Name, in.ParentID.id)
	if err != nil {
		folderError(w, err)
		return
	}
	writeAuthJSON(w, 200, folder)
}

func (h folderHandler) remove(w http.ResponseWriter, r *http.Request, owner string) {
	if h.Store == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	if err := h.Store.Delete(r.Context(), owner, r.PathValue("id")); err != nil {
		folderError(w, err)
		return
	}
	w.WriteHeader(204)
}

// place puts problems and posts in a folder, or makes them unsorted. It needs the folderId, so that a client that
// leaves it out cannot make them unsorted, and it answers alike for every id, so that nobody learns from it whether a
// problem or post of someone else exists.
func (h folderHandler) place(w http.ResponseWriter, r *http.Request, owner string) {
	if h.Store == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	var in placementBody
	if !contentJSON(w, r, &in) {
		return
	}
	if !in.FolderID.sent {
		authError(w, 400, "invalid_request")
		return
	}
	err := h.Store.Place(r.Context(), owner, in.FolderID.id, in.Items)
	if errors.Is(err, folders.ErrInvalid) {
		authError(w, 400, "invalid_request")
		return
	}
	if err != nil {
		folderError(w, err)
		return
	}
	w.WriteHeader(204)
}

func folderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, folders.ErrInvalid):
		authError(w, 400, "invalid_folder")
	case errors.Is(err, folders.ErrNotFound):
		authError(w, 404, "not_found")
	case errors.Is(err, folders.ErrLimit):
		authError(w, 409, "folder_limit")
	case errors.Is(err, folders.ErrDepth):
		authError(w, 409, "folder_depth")
	case errors.Is(err, folders.ErrNameTaken):
		authError(w, 409, "folder_name_taken")
	case errors.Is(err, folders.ErrCycle):
		authError(w, 409, "folder_cycle")
	default:
		authError(w, 503, "database_unavailable")
	}
}
