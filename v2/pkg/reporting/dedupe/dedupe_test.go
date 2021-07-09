package dedupe

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/yaklang/nuclei/v2/pkg/output"
	"github.com/stretchr/testify/require"
)

func TestDedupeDuplicates(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "nuclei")
	require.Nil(t, err, "could not create temporary storage")
	defer os.RemoveAll(tempDir)

	storage, err := New(tempDir)
	require.Nil(t, err, "could not create duplicate storage")

	tests := []*output.ResultEvent{
		{TemplateID: "test", Host: "https://example.com"},
		{TemplateID: "test", Host: "https://example.com"},
	}
	first, err := storage.Index(tests[0])
	require.Nil(t, err, "could not index item")
	require.True(t, first, "could not index valid item")

	second, err := storage.Index(tests[1])
	require.Nil(t, err, "could not index item")
	require.False(t, second, "could index duplicate item")
}
