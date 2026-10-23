package dashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"strconv"

	"github.com/thiemok/tiny-dash/api/internal/telemetry"
)

// The page's content hash travels like the next refresh time: the
// dashboard writes it into a meta tag (etagMeta), and the image handler
// derives the image's ETag from it. It hashes the data the page shows, not
// the rendered pixels, so the device can skip a panel refresh when nothing
// it would show has changed.

// etagMeta names the meta tag holding the page's content hash.
const etagMeta = "tinydash-etag"

// etagHexLen is the ETag's length in hex digits, without quotes. The device
// keeps the last one in a fixed slot of its request, so it never varies.
const etagHexLen = 16

// contentVersion identifies everything besides templateData that decides the
// pixels: the templates and the build (static assets, styles in Go). A
// deploy that changes the layout thus changes every ETag.
func contentVersion(fsys fs.FS) (string, error) {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00", telemetry.Version)
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", path, len(b))
		h.Write(b)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("hashing templates: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// contentHash hashes what the page shows: data, minus the next refresh
// time, which changes on every render without changing the pixels.
func (d templateData) contentHash(version string) (string, error) {
	d.NextRefresh = ""
	d.ETag = ""
	b, err := json.Marshal(d)
	if err != nil {
		return "", fmt.Errorf("encoding template data: %w", err)
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00", version)
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// imageETag is the quoted ETag of the packed image made from a page with
// content hash pageHash: the packing's color depth is not part of the page.
// It is empty without a page hash.
func imageETag(pageHash string, colorDepth int) string {
	if pageHash == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(pageHash + "\x00" + strconv.Itoa(colorDepth)))
	return `"` + hex.EncodeToString(sum[:])[:etagHexLen] + `"`
}
