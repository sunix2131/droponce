package receiverweb

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderEscapesFilename(t *testing.T) {
	var out bytes.Buffer
	err := Render(&out, PageData{
		FileName:     `<b>файл</b>.zip`,
		SizeBytes:    50 * 1024 * 1024 * 1024,
		Expires:      "12:30 05.07.2026",
		Remaining:    1,
		DownloadPath: "/d/token/download",
	})
	require.NoError(t, err)
	html := out.String()
	require.NotContains(t, html, `<b>файл</b>.zip`)
	require.Contains(t, html, `&lt;b&gt;файл&lt;/b&gt;.zip`)
	require.Contains(t, html, `50.0 GB`)
	require.Contains(t, html, `/d/token/download`)
}

func TestManifestHasInstallMetadata(t *testing.T) {
	body, err := ManifestJSON()
	require.NoError(t, err)
	require.Contains(t, string(body), `"short_name":"DropOnce"`)
	require.Contains(t, string(body), `"/drop-icon.svg"`)
}
