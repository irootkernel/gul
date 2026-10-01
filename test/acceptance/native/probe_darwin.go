//go:build darwin && !ios && !server

// Package native drives an isolated WebKit DOM from native test code only.
package native

/*
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdlib.h>
void gulAcceptanceProbe(void*, const char*);
*/
import "C"

import "unsafe"

func Probe(window unsafe.Pointer, script string) {
	value := C.CString(script)
	defer C.free(unsafe.Pointer(value))
	C.gulAcceptanceProbe(window, value)
}
