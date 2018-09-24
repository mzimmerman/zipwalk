package zipwalk

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	// "github.com/alexmullins/zip"
)

// SkipDir is used as a return value from WalkFuncs to indicate that
// the directory named in the call is to be skipped. It is not returned
// as an error by any function.
var SkipDir = filepath.SkipDir

// SkipZip allows you to skip going into the zip file
var SkipZip = fmt.Errorf("SkipZip")

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
type WalkFunc func(path string, info os.FileInfo, reader io.ReaderAt, err error) error

// Walk walks the file tree rooted at root including through zip files, calling walkFn for each file or
// directory in the tree, including root. All errors that arise visiting files
// and directories are filtered by walkFn. The real files are walked in lexical
// order, which makes the output deterministic but means that for very
// large directories Walk can be inefficient.  Files insize zip files are walked in the order they appear in the zip file.
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
			return walkFuncRecursive(filePath, info, f, walkFn, err)
		}
		return walkFn(filePath, info, f, nil)
	})
}

// ZipFileInfo is used to "mask" the modified time of the files extracted from the zip
type ZipFileInfo struct {
	os.FileInfo
	LastModified time.Time
}

// ModTime returns the date of the full parent zip file's modification time
func (zfi ZipFileInfo) ModTime() time.Time {
	return zfi.LastModified
}

// NewZipFileInfo creates an os.FileInfo from given last modified time and "parent" FileInfo
func NewZipFileInfo(lm time.Time, info os.FileInfo) ZipFileInfo {
	return ZipFileInfo{
		LastModified: lm,
		FileInfo:     info,
	}
}

func walkFuncRecursive(filePath string, info os.FileInfo, content io.ReaderAt, walkFn WalkFunc, err error) error {
	if err != nil {
		return fmt.Errorf("walkFuncRecursive received error when called for file %s - %v", filepath.Join(filePath, info.Name()), err)
	}
	err = walkFn(filePath, info, content, nil)
	if err == SkipZip {
		return nil
	}
	if err != nil {
		return fmt.Errorf("walkFuncRecursive received error from walkFn for file %s - %v", filepath.Join(filePath, info.Name()), err)
	}
	// is a zip file
	zr, err := zip.NewReader(content, info.Size())
	if err != nil {
		if strings.Contains(err.Error(), "zip: not a valid zip file") {
			log.Printf("File %s is not a valid zip file - %v", filepath.Join(filePath, info.Name()), err)
			return nil
		}
		return fmt.Errorf("walkFuncRecursive error reading file %s - %v", filepath.Join(filePath, info.Name()), err)
		// return walkFn(filePath, info, nil, err)
	}

	for _, f := range zr.File {
		// if !f.FileHeader.IsEncrypted() {
		rdr, err := f.Open()
		if err == nil {
			err = func() error {
				defer rdr.Close()
				insideContent, err := ioutil.ReadAll(rdr)
				if err != nil {
					if strings.Contains(err.Error(), "flate: corrupt input before offset") {
						log.Printf("File %s is likely encrypted - %v", filepath.Join(filePath, f.Name), err)
						return nil
					}
					if strings.Contains(err.Error(), "EOF") {
						log.Printf("File %s error reading file, got unexpected EOF - %v", filepath.Join(filePath, f.Name), err)
						return nil
					}
					return fmt.Errorf("Error reading file - %s - %v", filepath.Join(filePath, f.Name), err)
				}
				if strings.ToLower(filepath.Ext(f.Name)) == ".zip" {
					err = walkFuncRecursive(filepath.Join(filePath, f.Name), NewZipFileInfo(info.ModTime(), f.FileInfo()), bytes.NewReader(insideContent), walkFn, err)
					if err != nil {
						return fmt.Errorf("Received error from walkFuncRecursive - %s - %v", filepath.Join(filePath, f.Name), err)
					}
				} else {
					err = walkFn(filepath.Join(filePath, f.Name), NewZipFileInfo(info.ModTime(), f.FileInfo()), bytes.NewReader(insideContent), err)
					if err != nil {
						return fmt.Errorf("Received error from walkFn - %s - %v", filepath.Join(filePath, f.Name), err)
					}
				}
				return nil
			}()
			if err != nil {
				return err
			}
		} else { // err != nil
			if strings.Contains(err.Error(), "zip: unsupported") {
				log.Printf("File %s is likely corrupted - %v", filepath.Join(filePath, f.Name), err)
				return nil
			}
			return fmt.Errorf("Error opening file %s - %v", filepath.Join(filePath, f.Name), err)
		}
		// } else {
		// 	log.Printf("Ignoring encrypted file - %s", filepath.Join(filePath, f.Name))
		// }
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
