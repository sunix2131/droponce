package filesystem

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInspectOrdinaryUnicodeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unicode-файл.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o600))

	meta, err := Inspect(path)
	require.NoError(t, err)
	require.Equal(t, "unicode-файл.txt", meta.Name)
	require.Equal(t, int64(5), meta.SizeBytes)
	require.False(t, meta.IsSymlink)
	require.True(t, Unchanged(meta.ResolvedPath, meta.SizeBytes, meta.ModifiedAt))
}
