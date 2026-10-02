package deployment

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"
)

const LaunchdLabel = "xyz.rootkernel.gul.serve"

// LaunchdConfig describes a current-user agent. Home is the verified home
// directory of the same user that will load the agent. Logs and their parent
// must already exist with protected ownership and modes before rendering.
type LaunchdConfig struct {
	Home               string
	BinaryPath         string
	DataRoot           string
	Port               int
	TailnetHost        string
	DolgoraeExecutable string
	WorkspaceRoots     []string
	Policies           []string
}

// RenderLaunchdPlist renders a launchd user agent without installing it.
// The host must implement `gul serve --data-directory <absolute path>` and must
// verify the executable and data root independently before loading the agent.
func RenderLaunchdPlist(config LaunchdConfig) ([]byte, error) {
	if err := absoluteClean(config.Home); err != nil {
		return nil, fmt.Errorf("home: %w", err)
	}
	if err := absoluteClean(config.BinaryPath); err != nil {
		return nil, fmt.Errorf("binary: %w", err)
	}
	if err := absoluteClean(config.DataRoot); err != nil {
		return nil, fmt.Errorf("data root: %w", err)
	}
	if config.DataRoot != filepath.Join(config.Home, "Library", "Application Support", "Gul") {
		return nil, errors.New("data root must be the Gul Application Support directory")
	}
	logDir := filepath.Join(config.Home, "Library", "Logs", "Gul")
	stdout := filepath.Join(logDir, "stdout.log")
	stderr := filepath.Join(logDir, "stderr.log")
	for _, dir := range []string{config.Home, filepath.Join(config.Home, "Library"), filepath.Join(config.Home, "Library", "Logs")} {
		if err := ownedDirectory(dir, false); err != nil {
			return nil, fmt.Errorf("log parent: %w", err)
		}
	}
	if err := ownedDirectory(logDir, true); err != nil {
		return nil, fmt.Errorf("protected log directory: %w", err)
	}
	for _, path := range []string{stdout, stderr} {
		if err := protectedLogFile(path); err != nil {
			return nil, fmt.Errorf("protected log file: %w", err)
		}
	}
	arguments := []string{config.BinaryPath, "serve", "--data-directory", config.DataRoot}
	if config.Port != 0 {
		if config.Port < 1 || config.Port > 65535 {
			return nil, errors.New("invalid loopback port")
		}
		arguments = append(arguments, "--port", strconv.Itoa(config.Port))
	}
	if config.TailnetHost != "" {
		if _, _, err := ParseTailnetEndpoint(config.TailnetHost); err != nil {
			return nil, err
		}
		arguments = append(arguments, "--tailnet-host", config.TailnetHost)
	}
	if config.DolgoraeExecutable != "" {
		if err := absoluteClean(config.DolgoraeExecutable); err != nil {
			return nil, err
		}
		arguments = append(arguments, "--dolgorae-executable", config.DolgoraeExecutable)
	}
	for _, root := range config.WorkspaceRoots {
		if err := absoluteClean(root); err != nil {
			return nil, err
		}
		arguments = append(arguments, "--workspace-root", root)
	}
	for _, policy := range config.Policies {
		arguments = append(arguments, "--policy", policy)
	}
	values := append(arguments, stdout, stderr)
	for i, value := range values {
		var escaped bytes.Buffer
		if err := xml.EscapeText(&escaped, []byte(value)); err != nil {
			return nil, fmt.Errorf("escape plist value: %w", err)
		}
		values[i] = escaped.String()
	}
	var programArguments bytes.Buffer
	for _, value := range values[:len(arguments)] {
		fmt.Fprintf(&programArguments, "<string>%s</string>", value)
	}
	var plist bytes.Buffer
	fmt.Fprintf(&plist, `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array>%s</array>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><true/>
<key>ThrottleInterval</key><integer>30</integer>
<key>ProcessType</key><string>Background</string>
<key>Umask</key><integer>63</integer>
<key>StandardOutPath</key><string>%s</string>
<key>StandardErrorPath</key><string>%s</string>
</dict></plist>
`, LaunchdLabel, programArguments.String(), values[len(arguments)], values[len(arguments)+1])
	return plist.Bytes(), nil
}

func absoluteClean(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsRune(path, '\x00') || !utf8.ValidString(path) {
		return errors.New("path must be absolute and clean")
	}
	for _, r := range path {
		if r < 0x20 || r == 0x7f || r == 0xfffe || r == 0xffff {
			return errors.New("path contains an invalid XML character")
		}
	}
	return nil
}

func ownedDirectory(path string, private bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("directory is missing or is a symlink")
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(owner.Uid) != os.Getuid() {
		return errors.New("directory is not owned by the current user")
	}
	if private && info.Mode().Perm() != 0700 {
		return errors.New("log directory must have mode 0700")
	}
	if !private && info.Mode().Perm()&0022 != 0 {
		return errors.New("log parent is writable by another user")
	}
	return nil
}

func protectedLogFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return errors.New("log file must be regular with mode 0600")
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(owner.Uid) != os.Getuid() || owner.Nlink != 1 {
		return errors.New("log file has unsafe ownership or links")
	}
	return nil
}
