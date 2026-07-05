package application

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTokenServiceGenerates256BitURLSafeTokenAndHash(t *testing.T) {
	token, hash, err := NewTokenService().NewToken()
	require.NoError(t, err)
	require.Len(t, hash, 64)
	require.NotContains(t, token, "=")

	raw, err := base64.RawURLEncoding.DecodeString(token)
	require.NoError(t, err)
	require.Len(t, raw, 32)
	require.Equal(t, HashToken(token), hash)
}

func TestRegistryDoesNotReturnMutableQRCodeBytes(t *testing.T) {
	registry := NewRegistry()
	registry.Add(RuntimeTransfer{TransferID: "t1", RawToken: "secret", QRCodePNG: []byte{1, 2, 3}})

	first, ok := registry.Get("t1")
	require.True(t, ok)
	first.QRCodePNG[0] = 9

	second, ok := registry.GetByToken("secret")
	require.True(t, ok)
	require.Equal(t, byte(1), second.QRCodePNG[0])
}
