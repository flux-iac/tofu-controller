package storage

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func ArchiveDir(dir string) (out string, err error) {
	pwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	defer func() {
		if deferErr := os.Chdir(pwd); deferErr != nil && err == nil {
			err = deferErr
		}
	}()

	if err := os.Chdir(dir); err != nil {
		return "", err
	}

	dir = "./"
	if f, err := os.Stat(dir); os.IsNotExist(err) || !f.IsDir() {
		return "nil", fmt.Errorf("invalid dir path: %s", dir)
	}

	tf, err := os.CreateTemp("", "tf-")
	if err != nil {
		return "", err
	}
	defer tf.Close()

	tmpName := tf.Name()
	defer func() {
		if err != nil {
			os.Remove(tmpName)
		}
	}()

	h := sha1.New()
	mw := io.MultiWriter(h, tf)

	gw := gzip.NewWriter(mw)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	if err := filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if fi.IsDir() {
			return nil
		}

		header, err := tar.FileInfoHeader(fi, p)
		if err != nil {
			return err
		}
		// The name needs to be modified to maintain directory structure
		// as tar.FileInfoHeader only has access to the base name of the file.
		// Ref: https://golang.org/src/archive/tar/common.go?#L626
		relFilePath := p
		if filepath.IsAbs(dir) {
			relFilePath, err = filepath.Rel(dir, p)
			if err != nil {
				return err
			}
		}
		header.Name = relFilePath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := io.Copy(tw, f); err != nil {
			return err
		}

		return nil
	}); err != nil {
		return "", err
	}

	return tmpName, nil
}
