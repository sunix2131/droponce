package relay

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRelayUploadDownloadAndConsume(t *testing.T) {
	server, err := New(t.TempDir(), "")
	require.NoError(t, err)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("recipient_id", "phone-1"))
	require.NoError(t, writer.WriteField("expires_in_minutes", "10"))
	require.NoError(t, writer.WriteField("max_downloads", "1"))
	part, err := writer.CreateFormFile("file", "hello-файл.txt")
	require.NoError(t, err)
	_, err = part.Write([]byte("hello over relay"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/transfers", &body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var created createResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	require.NotEmpty(t, created.ShareURL)

	downloadResp, err := http.Get(created.ShareURL + "/download")
	require.NoError(t, err)
	defer downloadResp.Body.Close()
	require.Equal(t, http.StatusOK, downloadResp.StatusCode)
	data, err := io.ReadAll(downloadResp.Body)
	require.NoError(t, err)
	require.Equal(t, "hello over relay", string(data))

	again, err := http.Get(created.ShareURL)
	require.NoError(t, err)
	defer again.Body.Close()
	require.Equal(t, http.StatusNotFound, again.StatusCode)
}

func TestRelayReservesDownloadLimitAtomically(t *testing.T) {
	server, err := New(t.TempDir(), "")
	require.NoError(t, err)
	server.items["transfer"] = &item{
		ID:           "transfer",
		Token:        "token",
		Path:         t.TempDir() + "/file",
		MaxDownloads: 1,
		ExpiresAt:    time.Now().UTC().Add(time.Minute),
	}

	_, first := server.reserveDownload("transfer", "token")
	_, concurrent := server.reserveDownload("transfer", "token")

	require.True(t, first)
	require.False(t, concurrent)

	server.finishDownload("transfer", false)
	_, retry := server.reserveDownload("transfer", "token")
	require.True(t, retry)
}

func TestRelayRejectsFilesOverLimit(t *testing.T) {
	server, err := NewWithOptions(Options{Storage: t.TempDir(), MaxUploadBytes: 4})
	require.NoError(t, err)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("recipient_id", "phone-1"))
	require.NoError(t, writer.WriteField("expires_in_minutes", "10"))
	require.NoError(t, writer.WriteField("max_downloads", "1"))
	part, err := writer.CreateFormFile("file", "big.bin")
	require.NoError(t, err)
	_, err = part.Write([]byte("too large"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/transfers", &body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
}
