//go:build darwin

package tui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/image/tiff"
)

const readClipboardImageScript = `
on run argv
	set outPath to item 1 of argv
	try
		set imgData to the clipboard as «class PNGf»
		set imgKind to "png"
	on error
		try
			set imgData to the clipboard as «class TIFF»
			set imgKind to "tiff"
		on error
			return "NO_IMAGE"
		end try
	end try

	set outFile to POSIX file outPath
	set fileRef to open for access outFile with write permission
	try
		set eof of fileRef to 0
		write imgData to fileRef
		close access fileRef
	on error errMsg number errNum
		try
			close access fileRef
		end try
		error errMsg number errNum
	end try
	return imgKind
end run
`

func ReadClipboardImagePNG() (string, []byte, bool, error) {
	dir := clipboardImageDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", nil, false, err
	}
	path := filepath.Join(dir, "clipboard-"+time.Now().Format("20060102-150405")+"-"+randomHex(4)+".png")
	rawPath := path + ".raw"
	defer os.Remove(rawPath)

	kind, err := writeClipboardImageData(rawPath)
	if err != nil {
		return "", nil, false, err
	}
	if kind == "" {
		return "", nil, false, nil
	}

	switch kind {
	case "png":
		if err := os.Rename(rawPath, path); err != nil {
			return "", nil, false, err
		}
	case "tiff":
		if err := convertTIFFFileToPNG(rawPath, path); err != nil {
			return "", nil, false, err
		}
	default:
		return "", nil, false, fmt.Errorf("unexpected clipboard image kind %q", kind)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, false, err
	}
	return path, data, true, nil
}

func writeClipboardImageData(path string) (string, error) {
	cmd := exec.Command("/usr/bin/osascript", "-e", readClipboardImageScript, path)
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		if strings.Contains(trimmed, "NO_IMAGE") || strings.Contains(trimmed, "Can’t make") || strings.Contains(trimmed, "Can't make") {
			return "", nil
		}
		if trimmed == "" {
			return "", fmt.Errorf("osascript failed: %w", err)
		}
		return "", fmt.Errorf("osascript failed: %s", trimmed)
	}
	if trimmed == "NO_IMAGE" {
		return "", nil
	}
	return trimmed, nil
}

func convertTIFFFileToPNG(srcPath, dstPath string) error {
	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer in.Close()

	img, err := tiff.Decode(in)
	if err != nil {
		return err
	}

	out, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer out.Close()

	return png.Encode(out, img)
}

func clipboardImageDir() string {
	if info, err := os.Stat("/tmp"); err == nil && info.IsDir() {
		return filepath.Join("/tmp", "zot-clipboard-images")
	}
	return filepath.Join(os.TempDir(), "zot-clipboard-images")
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
