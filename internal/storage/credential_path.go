package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ResolveCredentialPath validates a logical key under an already selected
// credential root. Call it immediately before each authorized RPC; the caller
// still owns the provider authorization and trusted-root selection.
func ResolveCredentialPath(root, key string) (string, error) {
	if !filepath.IsAbs(root) || !validLogicalKey(key) {
		return "", errors.New("invalid credential root or logical key")
	}
	root = filepath.Clean(root)
	if err := validateOwnerOnlyComponent(root, true); err != nil {
		return "", err
	}
	current := root
	parts := strings.Split(key, "/")
	for index, part := range parts {
		current = filepath.Join(current, part)
		if err := validateOwnerOnlyComponent(current, index != len(parts)-1); err != nil {
			return "", err
		}
	}
	return current, nil
}

func validateOwnerOnlyComponent(filename string, directory bool) error {
	info, err := os.Lstat(filename)
	if err != nil {
		return err
	}
	return validateOwnerOnlyInfo(filename, info, directory, os.Getuid())
}

func validateOwnerOnlyInfo(filename string, info os.FileInfo, directory bool, uid int) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != uid || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unsafe owner-only component: %s", filename)
	}
	if directory {
		if !info.IsDir() || info.Mode().Perm() != 0700 {
			return fmt.Errorf("unsafe owner-only directory: %s", filename)
		}
	} else if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return fmt.Errorf("unsafe owner-only file: %s", filename)
	}
	return nil
}
