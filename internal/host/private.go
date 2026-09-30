// Package host composes one authenticated local core for shell and headless delivery.
package host

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

var ErrUnsafeState = errors.New("Gul host state is not protected; check ownership, permissions and symlinks")

func privateDirectory(root string) error {
	return checkPrivateDirectory(root, true)
}
func checkPrivateDirectory(root string, create bool) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return ErrUnsafeState
	}
	// Existing ancestors must be directories without symlinks. Only the selected
	// Gul state directory needs owner-only access; system/home ancestors retain
	// their ordinary permissions.
	for p := filepath.Dir(root); ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return ErrUnsafeState
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	if create {
		if err := os.Mkdir(root, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	f, err := privateOpen(root, unix.O_RDONLY|unix.O_DIRECTORY)
	if err != nil {
		return err
	}
	return f.Close()
}
func privateOpen(name string, flags int) (*os.File, error) {
	fd, err := unix.Open(name, flags|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, ErrUnsafeState
	}
	var s unix.Stat_t
	if unix.Fstat(fd, &s) != nil || int(s.Uid) != os.Getuid() || s.Mode&0077 != 0 || (s.Mode&unix.S_IFMT != unix.S_IFREG && s.Mode&unix.S_IFMT != unix.S_IFDIR) || (s.Mode&unix.S_IFMT == unix.S_IFREG && s.Nlink != 1) {
		unix.Close(fd)
		return nil, ErrUnsafeState
	}
	return os.NewFile(uintptr(fd), name), nil
}
func readPrivate(name string) ([]byte, error) {
	f, e := privateOpen(name, unix.O_RDONLY)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, 64*1024+1))
}
func writePrivate(name string, data []byte) error {
	if _, e := os.Lstat(name); e == nil {
		f, e := privateOpen(name, unix.O_RDONLY)
		if e != nil {
			return e
		}
		f.Close()
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(name), ".gul-state-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	e = errors.Join(e, f.Close())
	if e != nil {
		return e
	}
	return os.Rename(f.Name(), name)
}
func certificate(root string) (tls.Certificate, error) {
	filename := filepath.Join(root, "localhost.pem")
	var data []byte
	if _, e := os.Lstat(filename); errors.Is(e, os.ErrNotExist) {
		key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			return tls.Certificate{}, e
		}
		serial, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if e != nil {
			return tls.Certificate{}, e
		}
		now := time.Now()
		t := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Gul local host"}, NotBefore: now.Add(-time.Minute), NotAfter: now.AddDate(1, 0, 0), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
		der, e := x509.CreateCertificate(rand.Reader, t, t, &key.PublicKey, key)
		if e != nil {
			return tls.Certificate{}, e
		}
		encoded, e := x509.MarshalPKCS8PrivateKey(key)
		if e != nil {
			return tls.Certificate{}, e
		}
		data = append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded})...)
		if e = writePrivate(filename, data); e != nil {
			return tls.Certificate{}, e
		}
	} else {
		var e error
		data, e = readPrivate(filename)
		if e != nil {
			return tls.Certificate{}, e
		}
	}
	pair, e := tls.X509KeyPair(data, data)
	clear(data)
	if e != nil {
		return tls.Certificate{}, ErrUnsafeState
	}
	leaf, e := x509.ParseCertificate(pair.Certificate[0])
	if e != nil || leaf.VerifyHostname("127.0.0.1") != nil || time.Now().Before(leaf.NotBefore) || !time.Now().Before(leaf.NotAfter) {
		return tls.Certificate{}, errors.New("Gul local certificate is damaged or expired; rotate it while the core is stopped")
	}
	pair.Leaf = leaf
	return pair, nil
}
