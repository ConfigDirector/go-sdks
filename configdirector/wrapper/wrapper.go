// Package wrapper builds clients for the modules ConfigDirector ships on top of this SDK, such as
// the OpenFeature provider, so that each reports its own name and version to ConfigDirector in the
// SDK's place. Applications create their client with configdirector.NewClient instead.
package wrapper

import (
	"fmt"
	"regexp"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/ConfigDirector/go-sdks/configdirector/internal/identityseam"
)

var newClientIdentifiedAs = identityseam.NewClientIdentifiedAs.(func(identityseam.Identity, configdirector.Options) (*configdirector.Client, error))

var semanticVersionWithoutPrefix = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

// Module is one of the modules built on this SDK that reports its own name to ConfigDirector. The
// modules are the variables of this package; no other value is accepted.
type Module struct {
	sdkName string
}

// OpenFeatureProvider is the ConfigDirector OpenFeature provider for Go servers, module
// github.com/ConfigDirector/go-sdks/configdirector-openfeature.
var OpenFeatureProvider = Module{sdkName: "go-openfeature-server-provider"}

// NewClient creates a client that reports module at version to ConfigDirector in the SDK's place,
// and otherwise exactly as configdirector.NewClient creates one: version is the module's Version
// constant. It returns an error for a Module that is not one this package declares, a version that
// is not a semantic version without a prefix, such as 1.2.3 or 1.2.3-rc.1, or any error
// configdirector.NewClient returns for options.
func NewClient(module Module, version string, options configdirector.Options) (*configdirector.Client, error) {
	if module.sdkName == "" {
		return nil, fmt.Errorf("wrapper: the module is not a module this package declares; use one of its variables, such as wrapper.OpenFeatureProvider")
	}
	if !semanticVersionWithoutPrefix.MatchString(version) {
		return nil, fmt.Errorf("wrapper: the version %q is not a semantic version without a prefix, such as 1.2.3; pass the module's Version constant", version)
	}
	return newClientIdentifiedAs(identityseam.Identity{SDKName: module.sdkName, SDKVersion: version}, options)
}
