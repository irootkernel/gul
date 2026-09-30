//go:build darwin && !ios && !server

package desktop

/*
#cgo LDFLAGS: -framework Cocoa -framework WebKit -framework Security -framework Foundation
#include "native_https_darwin.h"
#include <stdlib.h>
*/
import "C"

import (
	"crypto/x509"
	"unsafe"
)

func installNativeHTTPS(window unsafe.Pointer, origin string, pin [32]byte, script string, der []byte) bool {
	if window == nil {
		return false
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return false
	}
	cOrigin := C.CString(origin)
	defer C.free(unsafe.Pointer(cOrigin))
	cScript := C.CString(script)
	defer C.free(unsafe.Pointer(cScript))
	return bool(C.gulInstallNativeHTTPS(window, cOrigin, (*C.uint8_t)(unsafe.Pointer(&pin[0])), cScript,
		C.int64_t(cert.NotBefore.Unix()), C.int64_t(cert.NotAfter.Unix())))
}

func probeNativeHTTPS(origin, candidate string, pin [32]byte, der []byte) bool {
	if len(der) == 0 {
		return false
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return false
	}
	cOrigin := C.CString(origin)
	defer C.free(unsafe.Pointer(cOrigin))
	cCandidate := C.CString(candidate)
	defer C.free(unsafe.Pointer(cCandidate))
	return bool(C.gulProbeNativeHTTPS(cOrigin, cCandidate,
		(*C.uint8_t)(unsafe.Pointer(&pin[0])),
		(*C.uint8_t)(unsafe.Pointer(&der[0])), C.int(len(der)),
		C.int64_t(cert.NotBefore.Unix()), C.int64_t(cert.NotAfter.Unix())))
}

func probeDelegateForwarding() bool { return bool(C.gulProbeDelegateForwarding()) }

func stopNativeApplication() { C.gulStopNativeApplication() }
