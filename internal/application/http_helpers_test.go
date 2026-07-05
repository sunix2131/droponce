package application

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContentDispositionSanitizesAndSupportsUnicode(t *testing.T) {
	header := contentDisposition("отчёт \"final\"\r\n.zip")
	require.NotContains(t, header, "\r")
	require.NotContains(t, header, "\n")
	require.Contains(t, header, `filename="download"`)
	require.Contains(t, header, "filename*=UTF-8''")
	require.True(t, strings.Contains(header, "%D0%BE"))
}

func TestContentTypeFallback(t *testing.T) {
	require.Equal(t, "application/octet-stream", contentType("file-without-extension"))
	require.Contains(t, contentType("photo.png"), "image/png")
}
