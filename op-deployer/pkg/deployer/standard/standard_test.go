package standard

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsKnownTag(t *testing.T) {
	for tag := range knownTags {
		require.True(t, IsKnownTag(tag))
	}
	require.False(t, IsKnownTag("unknown"))
}
