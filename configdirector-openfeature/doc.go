// Package configdirectoropenfeature is the ConfigDirector OpenFeature provider for Go servers.
//
// Create a Provider with NewProvider, register it with OpenFeature, and read values through an
// OpenFeature client:
//
//	provider, err := configdirectoropenfeature.NewProvider(configdirector.Options{SDKKey: os.Getenv("CONFIGDIRECTOR_SERVER_KEY")})
//	if err != nil {
//		log.Fatal(err)
//	}
//	if err := openfeature.SetProviderAndWait(provider); err != nil {
//		log.Fatal(err)
//	}
//	defer openfeature.Shutdown()
//
//	client := openfeature.NewClient("checkout")
//	user := openfeature.NewEvaluationContext("user-123", map[string]any{"traits": map[string]any{"plan": "pro"}})
//	if client.Boolean(ctx, "new-checkout", false, user) {
//		// ...
//	}
//
// The provider takes the same configdirector.Options as the SDK's client, so the connection mode,
// timeout, application metadata and telemetry are configured as they are for the SDK.
package configdirectoropenfeature
