package output

import (
	"os"
)

type Console struct{}

// NewConsole is the constructor of Console
func NewConsole() Console {
	return Console{}
}

// GetFileDescriptor provides the file descriptor used to write results
func (c Console) GetFileDescriptor(fileName *string) *os.File {
	return os.Stdout
}

// Close provides the required method to comply to the meta-object but does nothing
func (c Console) Close(fileDescriptor *os.File) error {
	return nil
}
