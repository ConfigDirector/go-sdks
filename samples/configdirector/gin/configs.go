package main

import (
	"net/http"
	"net/url"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/gin-gonic/gin"
)

func newRouter(client *configdirector.Client) *gin.Engine {
	router := gin.Default()

	router.GET("/configs", func(c *gin.Context) {
		configs, err := configsFor(client, userContextFrom(c.Request.URL.Query()))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, configs)
	})

	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Not found. Try GET /configs"})
	})

	return router
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
