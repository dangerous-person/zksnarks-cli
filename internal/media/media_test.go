package media_test

import (
	"encoding/json"
	"io/fs"
	"path"
	"strings"
	"testing"

	"github.com/dangerous-person/zksnarks-cli/internal/media"
	"github.com/dangerous-person/zksnarks-cli/internal/video"
)

func TestClips(t *testing.T) {
	var names []string
	fs.WalkDir(media.FS, ".", func(name string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(name, ".zkv") {
			names = append(names, name)
		}
		return err
	})
	if len(names) != 30 {
		t.Fatalf("%d clips: %v", len(names), names)
	}
	for _, name := range names {
		data, err := fs.ReadFile(media.FS, name)
		if err != nil {
			t.Fatal(err)
		}
		v, err := video.Read(data)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		cols, rows := v.Size()
		if len(data) > 64<<20 || cols > 512 || rows > 256 || v.Frames() > 20000 || v.FPS() < 1 || v.FPS() > 120 {
			t.Errorf("%s: %d bytes, %dx%d cells, %d frames at %v fps", name, len(data), cols, rows, v.Frames(), v.FPS())
		}
	}
}

func TestMontage(t *testing.T) {
	data, err := fs.ReadFile(media.FS, "trailer/montage.json")
	if err != nil {
		t.Fatal(err)
	}
	var mt struct {
		Sound string
		Title string
		Items []struct{ Clip string }
	}
	if err := json.Unmarshal(data, &mt); err != nil {
		t.Fatal(err)
	}
	if mt.Title == "" || len(mt.Items) == 0 {
		t.Fatalf("title %q, %d items", mt.Title, len(mt.Items))
	}
	names := []string{mt.Sound}
	for _, it := range mt.Items {
		names = append(names, it.Clip)
	}
	for _, name := range names {
		if _, err := fs.Stat(media.FS, path.Join("trailer", name)); err != nil {
			t.Error(err)
		}
	}
}
