package configdirector_test

import (
	"regexp"
	"testing"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/stretchr/testify/require"
)

var semanticVersionWithoutPrefix = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

func TestVersionIsASemanticVersionWithoutAPrefix(t *testing.T) {
	require.Regexp(t, semanticVersionWithoutPrefix, configdirector.Version)
}
