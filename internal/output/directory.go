package output

import (
	"fmt"
	"log/slog"
	"os"
)

type Directory struct {
	path string
}

// NewDirectory is the constructor of Directory. The given path is given to create the directory at construction time.
func NewDirectory(path string) (Directory, error) {
	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		slog.Error("NewDirectory", "message", err)
	}

	return Directory{
		path: path,
	}, err
}

// GetFileDescriptor provides the file descriptor used to write results as a file inside the directory's path.
func (d Directory) GetFileDescriptor(fileName *string) (result *os.File, err error) {
	file := fmt.Sprintf("%s/%s", d.path, *fileName)
	result, err = os.Create(file)
	if err != nil {
		slog.Error(err.Error())
	}
	slog.Info("GetFileDescriptor", "file", result.Name())

	return result, err
}

// Close closes the given file descriptor.
func (d Directory) Close(fileDescriptor *os.File) error {
	slog.Debug("Close", "file", fileDescriptor.Name())
	return fileDescriptor.Close()
}
