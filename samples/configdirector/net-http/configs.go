package main

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/ConfigDirector/go-sdks/configdirector"
)

func newRouter(client *configdirector.Client) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /configs", func(w http.ResponseWriter, r *http.Request) {
		configs, err := configsFor(client, userContextFrom(r.URL.Query()))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, configs)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Not found. Try GET /configs"})
	})

	return mux
}

func configsFor(client *configdirector.Client, user configdirector.UserContext) (map[string]any, error) {
	jsonValue := map[string]any{}
	if err := client.JSONValue("json-value-config", &jsonValue, user); err != nil {
		return nil, err
	}

	return map[string]any{
		"temporary-feature-flag": client.BoolValue("temporary-feature-flag", true, user),
		"permanent-kill-switch":  client.BoolValue("permanent-kill-switch", false, user),
		"integer-config":         client.IntValue("integer-config", 10, user),
		"day-of-the-week-config": client.StringValue("day-of-the-week-config", "Friday", user),
		"json-value-config":      jsonValue,
	}, nil
}

func userContextFrom(query url.Values) configdirector.UserContext {
	traits := map[string]any{}
	for parameter := range query {
		if !describesTheUser(parameter) {
			traits[parameter] = query.Get(parameter)
		}
	}

	return configdirector.UserContext{
		ID:        query.Get("id"),
		Name:      query.Get("name"),
		Traits:    traits,
		Anonymous: query.Get("anonymous") == "true",
	}
}

func describesTheUser(parameter string) bool {
	switch parameter {
	case "id", "name", "anonymous":
		return true
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
