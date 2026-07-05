package broker

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBrokerStoresOnlyOpaqueMessages(t *testing.T) {
	srv := New(Options{MaxSessionDuration: time.Minute, MaxInFlightBytes: 1024})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body, _ := json.Marshal(CreateSessionRequest{SessionID: "s1", ExpiresAt: time.Now().Add(time.Minute).Unix()})
	resp, err := http.Post(ts.URL+"/v1/sessions", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	msgBody, _ := json.Marshal(PostMessageRequest{From: "sender", To: "receiver", Type: "data", Body: "opaque-ciphertext"})
	resp, err = http.Post(ts.URL+"/v1/sessions/s1/messages", "application/json", bytes.NewReader(msgBody))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	resp, err = http.Get(ts.URL + "/v1/sessions/s1/messages?to=receiver&after=0")
	require.NoError(t, err)
	defer resp.Body.Close()
	var listed ListMessagesResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listed))
	require.Len(t, listed.Messages, 1)
	require.Equal(t, "opaque-ciphertext", listed.Messages[0].Body)
}

func TestBrokerRejectsInFlightLimit(t *testing.T) {
	srv := New(Options{MaxSessionDuration: time.Minute, MaxInFlightBytes: 4})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	body, _ := json.Marshal(CreateSessionRequest{SessionID: "s1", ExpiresAt: time.Now().Add(time.Minute).Unix()})
	resp, err := http.Post(ts.URL+"/v1/sessions", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	_ = resp.Body.Close()
	msgBody, _ := json.Marshal(PostMessageRequest{From: "sender", To: "receiver", Type: "data", Body: "too-large"})
	resp, err = http.Post(ts.URL+"/v1/sessions/s1/messages", "application/json", bytes.NewReader(msgBody))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
}
