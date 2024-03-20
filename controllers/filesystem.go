package controllers

import (
	"io"
	"os"
	"path"
)

type Filesystem interface {
	GetWriter(filePath string) (io.WriteCloser, error)
	GetReader(filePath string) (io.ReadCloser, error)
}

type LocalFilesystem struct {
	root string
}

func NewLocalFilesystem(root string) *LocalFilesystem {
	return &LocalFilesystem{root: root}
}

func (fs *LocalFilesystem) GetWriter(filePath string) (io.WriteCloser, error) {
	return os.Create(path.Join(fs.root, filePath))
}
func (fs *LocalFilesystem) GetReader(filePath string) (io.ReadCloser, error) {
	return os.Open(path.Join(fs.root, filePath))
}
