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
package configdirector
