package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// push uploads the images that the statement and the editorial reference by a path inside the problem directory and
// saves the draft with their URLs; the local files keep the paths, so a local preview still shows them.

// The destination of a Markdown image: bare, or in angle brackets when it holds spaces.
var imageRef = regexp.MustCompile(`!\[(?:[^\]\\\n]|\\.)*\]\(\s*(<[^>\n]*>|[^\s()<>]+)(?:\s+(?:"[^"\n]*"|'[^'\n]*'))?\s*\)`)
var codeSpan = regexp.MustCompile("`+[^`\n]*`+")
var codeFence = regexp.MustCompile("^ {0,3}(```|~~~)")
var urlScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)

// The limits of POST /my/images, which accepts no WebP and does not resize.
const maxImageBytes = 512 << 10
const maxImageSide = 2048

// localImage returns the file an image destination names, unless it is a URL or a path on the site.
func localImage(destination string) (string, bool) {
	destination = strings.TrimSuffix(strings.TrimPrefix(destination, "<"), ">")
	if destination == "" || urlScheme.MatchString(destination) || strings.HasPrefix(destination, "/") || strings.HasPrefix(destination, "#") {
		return "", false
	}
	path, err := url.PathUnescape(destination)
	if err != nil {
		return "", false
	}
	return path, true
}

// rewriteImages replaces the destination of each local image outside code with what replace returns for its path;
// an empty answer leaves the image as written.
func rewriteImages(text string, replace func(path string) (string, error)) (string, error) {
	lines := strings.SplitAfter(text, "\n")
	fence := ""
	for i, line := range lines {
		if marker := codeFence.FindStringSubmatch(line); marker != nil {
			if fence == "" {
				fence = marker[1]
			} else if fence == marker[1] {
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		spans := codeSpan.FindAllStringIndex(line, -1)
		var out strings.Builder
		last := 0
		for _, m := range imageRef.FindAllStringSubmatchIndex(line, -1) {
			inCode := false
			for _, span := range spans {
				inCode = inCode || (m[0] < span[1] && span[0] < m[1])
			}
			path, local := localImage(line[m[2]:m[3]])
			if inCode || !local {
				continue
			}
			to, err := replace(path)
			if err != nil {
				return "", err
			}
			if to == "" {
				continue
			}
			out.WriteString(line[last:m[2]])
			out.WriteString(to)
			last = m[3]
		}
		out.WriteString(line[last:])
		lines[i] = out.String()
	}
	return strings.Join(lines, ""), nil
}

// readImage returns a local image that the server would accept. A path with no file answers os.ErrNotExist.
func readImage(root *os.Root, path string) ([]byte, error) {
	if !filepath.IsLocal(filepath.FromSlash(path)) {
		return nil, fmt.Errorf("image path must stay inside the problem directory: %q", path)
	}
	invalid := fmt.Errorf("image %s: requires a PNG or JPEG file of at most %d KiB and %dx%d pixels", path, maxImageBytes>>10, maxImageSide, maxImageSide)
	f, err := root.Open(filepath.FromSlash(path))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxImageBytes {
		return nil, invalid
	}
	data, err := io.ReadAll(io.LimitReader(f, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") || config.Width > maxImageSide || config.Height > maxImageSide {
		return nil, invalid
	}
	return data, nil
}

// imageNotes checks every local image of text and names the ones without a file, which push leaves as written.
func imageNotes(root *os.Root, name, text string) ([]string, error) {
	var notes []string
	_, err := rewriteImages(text, func(path string) (string, error) {
		if _, err := readImage(root, path); errors.Is(err, os.ErrNotExist) {
			notes = append(notes, fmt.Sprintf("%s: image %q is not a file in the problem directory; push leaves it as written", name, path))
		} else if err != nil {
			return "", err
		}
		return "", nil
	})
	return notes, err
}

var imageURL = regexp.MustCompile(`^/api/images/[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)

// uploadImages replaces the local images of text with their URLs. known maps the SHA-256 of a file to the URL of an
// earlier upload, so an unchanged image is not sent again; the server also answers a repeated image with the same URL.
func (c *apiClient) uploadImages(ctx context.Context, root *os.Root, text string, known map[string]string) (string, error) {
	return rewriteImages(text, func(path string) (string, error) {
		data, err := readImage(root, path)
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		digest := fmt.Sprintf("%x", sha256.Sum256(data))
		if to := known[digest]; imageURL.MatchString(to) {
			return to, nil
		}
		var result struct {
			URL string `json:"url"`
		}
		if err := c.call(ctx, "POST", "/my/images", map[string]string{"data": base64.StdEncoding.EncodeToString(data)}, &result, true); err != nil {
			return "", fmt.Errorf("image %s: %w", path, err)
		}
		if !imageURL.MatchString(result.URL) {
			return "", errors.New("invalid image URL from server")
		}
		known[digest] = result.URL
		return result.URL, nil
	})
}
