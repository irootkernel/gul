package files

import (
	"bytes"
	"errors"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

func descriptorPath(file *os.File) (string, error) {
	buffer := make([]byte, 1024) // Darwin MAXPATHLEN, required by F_GETPATH.
	_, _, errno := syscall.Syscall(unix.SYS_FCNTL, file.Fd(), uintptr(unix.F_GETPATH), uintptr(unsafe.Pointer(&buffer[0])))
	runtime.KeepAlive(buffer)
	if errno != 0 {
		return "", errno
	}
	end := bytes.IndexByte(buffer, 0)
	if end < 0 {
		return "", errors.New("descriptor path is not terminated")
	}
	return string(buffer[:end]), nil
}
