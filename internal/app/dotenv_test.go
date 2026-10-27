package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDotEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := `# Scaleway
SCW_REGISTRY="rg.fr-par.scw.cloud"

UAC_BLUESKY_HANDLE="handle.bsky.social"
UAC_MASTODON_SERVER='https://mastodon.social'
export UAC_MISTRAL_MODEL=mistral-large-2512
UAC_BLUESKY_PASSWORD="from-file"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	// Unset the keys the file sets, then set one that must win over the file
	for _, key := range []string{"SCW_REGISTRY", "UAC_BLUESKY_HANDLE", "UAC_MASTODON_SERVER", "UAC_MISTRAL_MODEL"} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
	t.Setenv("UAC_BLUESKY_PASSWORD", "from-environment")

	require.NoError(t, LoadDotEnv(path))

	assert.Equal(t, "rg.fr-par.scw.cloud", os.Getenv("SCW_REGISTRY"))
	assert.Equal(t, "handle.bsky.social", os.Getenv("UAC_BLUESKY_HANDLE"))
	assert.Equal(t, "https://mastodon.social", os.Getenv("UAC_MASTODON_SERVER"))
	assert.Equal(t, "mistral-large-2512", os.Getenv("UAC_MISTRAL_MODEL"))
	assert.Equal(t, "from-environment", os.Getenv("UAC_BLUESKY_PASSWORD"))
}

func TestLoadDotEnv_MissingFile(t *testing.T) {
	assert.NoError(t, LoadDotEnv(filepath.Join(t.TempDir(), ".env")))
}

func TestLoadDotEnv_MalformedLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	require.NoError(t, os.WriteFile(path, []byte("UAC_OK=1\nnot a pair\n"), 0o600))

	err := LoadDotEnv(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ":2:")
}
