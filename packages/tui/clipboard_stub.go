//go:build !darwin

package tui

func ReadClipboardImagePNG() (string, []byte, bool, error) {
	return "", nil, false, nil
}
