package render_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pantafive/embisto/render"
)

func TestExampleTemplatesLoad(t *testing.T) {
	loaded, err := render.LoadTemplates(os.DirFS("../examples/templates"))

	require.NoError(t, err)
	require.Len(t, loaded, 4)
}
