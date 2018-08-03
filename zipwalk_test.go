package zipwalk_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mzimmerman/zipwalk"
)

func TestOpen(t *testing.T) {
	tests := []struct {
		Name        string
		ExpectError bool
	}{
		{"testdata/a.txt", false},
		{"testdata/a.zip", false},
		{"testdata/nf.txt", true},
		{"testdata/a.zip/a.txt", false},
		{"testdata/a.zip/nf.txt", true},
		{"testdata/nf.zip/nf.txt", true},
		{"testdata/a.zip/dir1.zip", false},
		{"testdata/a.zip/dir1.zip/dir1/dir1.txt", false},
		{"testdata/a.zip/dir1.zip/dir1/nf.txt", true},
		{"testdata/a.zip/dir1.zip/dir2/nf.txt", true},
	}
	for _, val := range tests {
		f, err := zipwalk.Open(val.Name)
		if err == nil {
			err = f.Close()
			if err != nil {
				t.Errorf("Error closing zip file - %s", err)
			}
		}
		if err != nil && !val.ExpectError {
			t.Errorf("Error unexpected opening %s", val.Name)
		}
		if err == nil && val.ExpectError {
			t.Errorf("Expected error but didn't get one - %s", val.Name)
		}
	}
}

func TestWalk(t *testing.T) {
	expectedPaths := map[string]struct{}{
		"testdata":                                    struct{}{},
		"testdata/a.txt":                              struct{}{},
		"testdata/a.zip/a.txt":                        struct{}{},
		"testdata/a.zip/dir1.zip":                     struct{}{},
		"testdata/a.zip/dir1.zip/dir1":                struct{}{},
		"testdata/a.zip/dir1.zip/dir1/dir1.txt":       struct{}{},
		"testdata/a.zip/b.zip":                        struct{}{},
		"testdata/a.zip/b.zip/a.txt":                  struct{}{},
		"testdata/a.zip/b.zip/dir1.zip":               struct{}{},
		"testdata/a.zip/b.zip/dir1.zip/dir1":          struct{}{},
		"testdata/a.zip/b.zip/dir1.zip/dir1/dir1.txt": struct{}{},
		"testdata/a.zip":                              struct{}{},
		"testdata/dir2.zip/dir1":                      struct{}{},
		"testdata/dir2.zip/dir1/dir1.txt":             struct{}{},
		"testdata/dir2.zip":                           struct{}{},
	}
	err := zipwalk.Walk("testdata", func(path string, info os.FileInfo, reader io.Reader, err error) error {
		if err != nil {
			t.Errorf("Error walking testdata - %v", err)
		}
		path = filepath.ToSlash(path)
		if _, ok := expectedPaths[path]; ok {
			t.Logf("Walked path %s", path)
			delete(expectedPaths, path)
		} else {
			t.Errorf("Got unexpected path - %s", path)
		}
		return nil
	})
	if err != nil {
		t.Errorf("Error walking testdata - %v", err)
	}
	for k := range expectedPaths {
		t.Errorf("Expected path not traversed - %s", k)
	}
}
