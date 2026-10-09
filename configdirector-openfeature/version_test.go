package configdirectoropenfeature_test

import (
	"regexp"
	"testing"

	configdirectoropenfeature "github.com/ConfigDirector/go-sdks/configdirector-openfeature"
	"github.com/stretchr/testify/require"
)

var semanticVersionWithoutPrefix = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

func TestVersionIsASemanticVersionWithoutAPrefix(t *testing.T) {
	require.Regexp(t, semanticVersionWithoutPrefix, configdirectoropenfeature.Version)
}
