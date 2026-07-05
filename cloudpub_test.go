package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCloudPubURLParsesPublishedURL(t *testing.T) {
	output := "Сервис опубликован: [DropOnce] http://127.0.0.1:9876 -> https://promptly-exalted-gar.cloudpub.ru:443"
	require.Equal(t, "https://promptly-exalted-gar.cloudpub.ru:443", cloudPubURL(output))
}
