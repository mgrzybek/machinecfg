package ports

import (
	"os"
)

// Output is an abstract object used to export the data outside of the program
type Output interface {
	// GetFileDescriptor provides the file descriptor used to write results
	GetFileDescriptor(fileName *string) (*os.File, error)

	// Close closes the opened file descriptor in a clean way
	Close(fileDescriptor *os.File) error
}
