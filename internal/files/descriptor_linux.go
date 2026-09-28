package files

import (
	"fmt"
	"os"
)

func descriptorPath(file *os.File) (string, error) {
	return os.Readlink(fmt.Sprintf("/proc/self/fd/%d", file.Fd()))
}
