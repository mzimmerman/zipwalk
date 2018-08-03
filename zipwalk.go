package zipwalk

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
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
			content, err := ioutil.ReadAll(f)
			return walkFuncRecursive(filePath, info, content, walkFn, err)
		}
		return walkFn(filePath, info, f, nil)
	})
}

func walkFuncRecursive(filePath string, info os.FileInfo, content []byte, walkFn WalkFunc, err error) error {
	if err != nil {
		return err
	}
	err = walkFn(filePath, info, bytes.NewReader(content), nil)
	if err != nil {
		return err
	}
	// is a zip file
	zr, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return walkFn(filePath, info, nil, err)
	}
	for _, f := range zr.File {
		rdr, err := f.Open()
		closeIt := err == nil
		if strings.ToLower(filepath.Ext(f.Name)) == ".zip" {
			content, err := ioutil.ReadAll(rdr)
			err = walkFuncRecursive(filepath.Join(filePath, f.Name), f.FileInfo(), content, walkFn, err)
		} else {
			err = walkFn(filepath.Join(filePath, f.Name), f.FileInfo(), bytes.NewReader(content), err)
		}
		if closeIt {
			rdr.Close()
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Stat will get the status of files embedded in a zip path
// e.g., file1.zip/file2.zip/a.txt
func Stat(path string) (os.FileInfo, error) {
	path = filepath.ToSlash(filepath.Clean(path))
	firstZipLoc := strings.Index(strings.ToLower(filepath.ToSlash(path)), ".zip/")
	if firstZipLoc == -1 {
		return os.Stat(path)
	}
	curLoc := firstZipLoc + 4
	firstZip, err := zip.OpenReader(path[:curLoc])
	if err != nil {
		return nil, fmt.Errorf("error opening zip file - %s", path)
	}
	defer firstZip.Close()
	return statRecursive(&firstZip.Reader, path[curLoc+1:])
}

func statRecursive(zf *zip.Reader, path string) (os.FileInfo, error) {
	fileToFind := path
	nextZipLoc := strings.Index(strings.ToLower(filepath.ToSlash(path)), ".zip/")
	if nextZipLoc != -1 {
		fileToFind = path[:nextZipLoc+4]
	}
	for _, f := range zf.File {
		if f.Name == fileToFind {
			if nextZipLoc == -1 {
				return f.FileInfo(), nil
			}
			fopen, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("Error opening the file we wanted to find - %s - %v", path, err)
			}
			buf, err := ioutil.ReadAll(fopen)
			fopen.Close()
			if err != nil {
				return nil, fmt.Errorf("Error reading zip file - %s - %v", path, err)
			}
			zr, err := zip.NewReader(bytes.NewReader(buf), int64(len(buf)))
			if err != nil {
				return nil, fmt.Errorf("Error opening zip file - %s - %v", path, err)
			}
			return statRecursive(zr, path[len(fileToFind)+1:])
		}
	}
	return nil, os.ErrNotExist
}
