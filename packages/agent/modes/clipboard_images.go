package modes

import (
	"strings"

	"github.com/patriceckhart/zot/packages/provider"
)

type clipboardImageAttachment struct {
	Marker string
	Image  provider.ImageBlock
}

func preparePromptWithClipboardImages(text string, pending []clipboardImageAttachment) (string, []provider.ImageBlock) {
	out := text
	images := make([]provider.ImageBlock, 0, len(pending))
	for _, item := range pending {
		if item.Marker == "" || !strings.Contains(out, item.Marker) {
			continue
		}
		out = strings.ReplaceAll(out, item.Marker, " ")
		images = append(images, item.Image)
	}
	out = strings.Join(strings.Fields(out), " ")
	return out, images
}
