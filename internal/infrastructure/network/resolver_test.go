package network

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsPrivateIPv4(t *testing.T) {
	require.True(t, IsPrivateIPv4("10.0.0.1"))
	require.True(t, IsPrivateIPv4("172.16.10.2"))
	require.True(t, IsPrivateIPv4("192.168.1.20"))
	require.False(t, IsPrivateIPv4("127.0.0.1"))
	require.False(t, IsPrivateIPv4("0.0.0.0"))
	require.False(t, IsPrivateIPv4("169.254.1.1"))
	require.False(t, IsPrivateIPv4("8.8.8.8"))
	require.False(t, IsPrivateIPv4("::1"))
}
