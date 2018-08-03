package zipwalk

import (
	"archive/zip"
	"bytes"
	"io"
	"io/ioutil"
	"log"
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
			log.Printf("isdir - %s", filePath)
			return walkFn(filePath, info, nil, err)
		}
		f, err := os.Open(filePath)
		if err != nil {
			log.Printf("osopenerror - %s", filePath)
			return walkFn(filePath, info, nil, err)
		}
		defer f.Close()
		ext := strings.ToLower(filepath.Ext(filePath))
		log.Printf("ext for %s =  %s", filePath, ext)
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
		log.Printf("zipnewreadererror - %s", filePath)
		return walkFn(filePath, info, nil, err)
	}
	for _, f := range zr.File {
		rdr, err := f.Open()
		closeIt := err == nil
		log.Printf("Looking through %s for %s", filePath, f.Name)
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
			log.Printf("errorafterwalk - %s", filePath)
			return err
		}
	}
	return nil
}

// Open will open files inside zip files given a full path
// e.g., file1.zip/file2.zip/a.txt
func Open(path string) (*Reader, error) {
	path = filepath.Clean(path)
	rdr := &Reader{}
	firstZipLoc := strings.Index(strings.ToLower(filepath.ToSlash(path)), ".zip/")
	if firstZipLoc == -1 {
		f, err := os.Open(path)
		rdr.file = f
		return rdr, err
	}
	curLoc := firstZipLoc + 4
	firstZip, err := zip.OpenReader(path[:curLoc])
	if err != nil {
		return rdr, nil
	}
	currentZip := &firstZip.Reader
	rdr.parent = firstZip
NextZipInPath:
	for {
		nextZipLoc := strings.Index(strings.ToLower(filepath.ToSlash(path[curLoc:])), ".zip/")
		if nextZipLoc == -1 {
			fileToFind := path[curLoc+1:]
			for _, f := range currentZip.File {
				if f.Name == fileToFind {
					fopen, err := f.Open()
					if err != nil {
						rdr.Close()
						return rdr, err
					}
					rdr.file = fopen
					return rdr, nil
				}
			}
			rdr.Close()
			return nil, os.ErrNotExist
		}
		fileToFind := path[curLoc+1 : curLoc+1+nextZipLoc]
		for _, f := range currentZip.File {
			if f.Name == fileToFind {
				fopen, err := f.Open()
				if err != nil {
					rdr.Close()
					return nil, err
				}
				zipContents, err := ioutil.ReadAll(fopen)
				if err != nil {
					rdr.Close()
					return nil, err
				}
				fopen.Close()
				nextZip, err := zip.NewReader(bytes.NewReader(zipContents), f.FileInfo().Size())
				if err != nil {
					rdr.Close()
					return nil, err
				}
				curLoc = curLoc + 1 + nextZipLoc + 1
				currentZip = nextZip
				continue NextZipInPath
			}
		}
		rdr.Close()
		return nil, os.ErrNotExist
	}
}

type Reader struct {
	file   io.ReadCloser
	parent *zip.ReadCloser
}

// Close the reader and any underlyzing zip file
func (r *Reader) Close() error {
	var firstError error
	if r.file != nil {
		firstError = r.file.Close()
	}
	if r.parent != nil {
		err := r.parent.Close()
		if err != nil && firstError == nil {
			firstError = err
		}
	}
	return firstError
}
