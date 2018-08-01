package zipwalk

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SkipDir is used as a return value from WalkFuncs to indicate that
// the directory named in the call is to be skipped. It is not returned
// as an error by any function.
var SkipDir = filepath.SkipDir

// WalkFunc is the type of the function called for each file or directory
// visited by Walk. The path argument contains the argument to Walk as a
// prefix; that is, if Walk is called with "dir", which is a directory
// containing the file "a", the walk function will be called with argument
// "dir/a". The info argument is the os.FileInfo for the named path.
//
// If there was a problem walking to the file or directory named by path, the
// incoming error will describe the problem and the function can decide how
// to handle that error (and Walk will not descend into that directory). If
// an error is returned, processing stops. The sole exception is when the function
// returns the special value SkipDir. If the function returns SkipDir when invoked
// on a directory, Walk skips the directory's contents entirely.
// If the function returns SkipDir when invoked on a non-directory file,
// Walk skips the remaining files in the containing directory.
type WalkFunc func(path string, info os.FileInfo, reader io.Reader, err error) error

// Walk walks the file tree rooted at root including through zip files, calling walkFn for each file or
// directory in the tree, including root. All errors that arise visiting files
// and directories are filtered by walkFn. The real files are walked in lexical
// order, which makes the output deterministic but means that for very
// large directories Walk can be inefficient.  Zip files are walked in the order they appear zipped.
// Walk does not follow symbolic links.
func Walk(root string, walkFn WalkFunc) error {
	return filepath.Walk(root, func(filePath string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return walkFn(filePath, info, nil, err)
		}
		f, err := os.Open(filePath)
		if err != nil {
			return walkFn(filePath, info, nil, err)
		}
		defer f.Close()
		if strings.ToLower(filepath.Ext(filePath)) == ".zip" {
			zr, err := zip.NewReader(f, info.Size())
			if err != nil {
				return walkFn(filePath, info, nil, err)
			}
			for _, f := range zr.File {
				rdr, err := f.Open()
				closeIt := err == nil
				err = walkFn(filepath.Join(filePath, f.Name), f.FileInfo(), rdr, err)
				if closeIt {
					rdr.Close()
				}
				if err != nil {
					return err
				}
			}
		}
		return walkFn(filePath, info, f, nil)
	})
}
