package file

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Type int

var (
	TypeDir  = Type(1)
	TypeFile = Type(2)
	TypeAll  = TypeDir | TypeFile
)

// IsDir checks whether the path is directory or not, returns true if directory.
func IsDir(path string) bool {
	s, err := os.Stat(path)
	return err == nil && s.IsDir()
}

// IsFile checks whether the path is file or not, returns true if file.
func IsFile(path string) bool {
	s, err := os.Stat(path)
	return err == nil && !s.IsDir()
}

// IsExist checks whether a file or directory exists, returns true if the file or directory exists.
func IsExist(path string) bool {
	_, err := os.Stat(path)
	return err == nil || os.IsExist(err)
}

// CopyFile copies a file from source to dest. Any existing file will be overwritten
// and attributes will be ignored.
func CopyFile(src string, dst string) (err error) {
	var srcInfo fs.FileInfo
	if srcInfo, err = os.Stat(src); err != nil || !srcInfo.Mode().IsRegular() {
		return err
	}

	srcFh, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func(srcFh *os.File) {
		_ = srcFh.Close()
	}(srcFh)

	// if dst already exists and is a directory, set destination filename equals the source src filename.
	if IsDir(dst) {
		dst = filepath.Join(dst, srcInfo.Name())
	} else {
		err = os.MkdirAll(filepath.Dir(dst), 0750)
		if err != nil {
			return err
		}
	}

	dstFh, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func(dstFh *os.File) {
		_ = dstFh.Close()
	}(dstFh)

	var size int64
	if size, err = io.Copy(dstFh, srcFh); err != nil || size != srcInfo.Size() {
		err = fmt.Errorf("copy failed: %d of %d, err: %w", size, srcInfo.Size(), err)
	}
	return err
}

// CopyDir recursively copies all files from src to dst，attributes will be ignored.
func CopyDir(src string, dst string) (err error) {
	if list, err := ListFiles(src, TypeFile); err == nil {
		for _, srcFile := range list {
			stripSrcDir := strings.TrimPrefix(srcFile, src)
			dstFile := filepath.Join(dst, stripSrcDir)
			if err = CopyFile(srcFile, dstFile); err != nil {
				return err
			}
		}
	}
	return err
}

// CreateIfNotExists creates a file or a directory only if it does not already exist.
func CreateIfNotExists(path string, isFile bool) (err error) {
	if exist := IsExist(path); !exist {
		if !isFile {
			return os.MkdirAll(path, 0755)
		}
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		var f *os.File
		f, err = os.OpenFile(path, os.O_CREATE, 0755)
		if err != nil {
			return err
		}
		defer func(f *os.File) {
			_ = f.Close()
		}(f)
	}
	return err
}

// InfoStr returns the file info json string, if not exist, return ""
func InfoStr(file string) string {
	s, err := os.Stat(file)
	if err != nil {
		return ""
	}
	type fileInfo struct {
		Name    string
		Size    int64
		Mode    uint32
		ModeStr string
		ModTime time.Time
		IsDir   bool
	}
	info := fileInfo{
		Name:    s.Name(),
		Size:    s.Size(),
		Mode:    uint32(s.Mode()),
		ModeStr: s.Mode().String(),
		ModTime: s.ModTime().UTC(),
		IsDir:   s.IsDir(),
	}
	bts, _ := json.Marshal(info)
	return string(bts)
}

// ListFiles recursively returns all files or dirs in the directory.
//
//	fileType: TypeDir =1, returns all dirs in the given directory.
//	fileType: TypeFile=2, returns all files in the given directory.
//	fileType: TypeAll =3, returns all files and dirs in the given directory.
//	fileType: others,     returns all files in the given directory.
func ListFiles(dir string, fileType Type) (files []string, err error) {
	fn := func(path string, d os.DirEntry, e error) error {
		if e == nil {
			switch fileType & TypeAll {
			case TypeDir:
				if !d.IsDir() {
					return nil
				}
			case TypeFile:
				if d.IsDir() {
					return nil
				}
			case TypeAll:
			default:
				if d.IsDir() {
					return nil
				}
			}
			files = append(files, path)
		}
		return e
	}

	err = filepath.WalkDir(dir, fn)
	sort.Slice(files, func(i, j int) bool {
		return files[i] < files[j]
	})
	return files, err
}
