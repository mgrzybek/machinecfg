package output

import (
	"fmt"
	"log/slog"
	"os"
)

type Directory struct {
	path   string
	logger *slog.Logger
}

// NewDirectory is the constructor of Directory. The given path is given to create the directory at construction time.
func NewDirectory(logger *slog.Logger, path string) (Directory, error) {
	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		slog.Error("NewDirectory", "message", err)
	}

	return Directory{
		logger: logger,
		path:   path,
	}, err
}

// GetFileDescriptor provides the file descriptor used to write results as a file inside the directory's path.
func (d Directory) GetFileDescriptor(fileName *string) (result *os.File) {
	file := fmt.Sprintf("%s/%s", d.path, *fileName)
	result, _ = os.Create(file)
	d.logger.Info("GetFileDescriptor", "file", result.Name())

	return result
}

// Close closes the given file descriptor.
func (d Directory) Close(fileDescriptor *os.File) error {
	d.logger.Debug("Close", "file", fileDescriptor.Name())
	return fileDescriptor.Close()
}
