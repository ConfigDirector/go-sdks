// Package configdirector is the ConfigDirector server SDK for Go.
//
// Create one Client for the application with NewClient, wait for its first config state with
// WaitForReady, read configs with the typed methods, and close it on shutdown:
//
//	client, err := configdirector.NewClient(configdirector.Options{SDKKey: os.Getenv("CONFIGDIRECTOR_SDK_KEY")})
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer client.Close()
//
//	if err := client.WaitForReady(ctx); err != nil {
//		log.Printf("config state has not arrived yet, reads return their default value: %v", err)
//	}
//
//	user := configdirector.UserContext{ID: "user-123", Traits: map[string]any{"plan": "pro"}}
//	if client.BoolValue("new-checkout", false, user) {
//		// ...
//	}
//
// Every read takes an in-code default value and returns it, rather than an error, when the
// config cannot be read: the *ValueDetails methods say why.
//
// To act on config state as it changes, watch one key or register for the client's events:
//
//	unwatch := client.WatchBool("new-checkout", false, user, func(enabled bool) {
//		// called with the new value whenever the config changes
//	})
//	defer unwatch()
//
//	client.OnConfigsUpdated(func(update configdirector.ConfigsUpdated) {
//		log.Printf("configs updated: %v", update.Keys)
//	})
//
// To test code that takes a *Client without a server, build the client with package
// configdirectortest, which serves values a test controls.
package configdirector
