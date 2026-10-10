package main

import (
	"context"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/open-feature/go-sdk/openfeature"
)

func newRouter(client *openfeature.Client) *gin.Engine {
	router := gin.Default()

	router.GET("/configs", func(c *gin.Context) {
		c.JSON(http.StatusOK, configsFor(c.Request.Context(), client, evaluationContextFrom(c.Request.URL.Query())))
	})

	router.GET("/configs/:key", func(c *gin.Context) {
		c.JSON(http.StatusOK, booleanDetailsFor(c.Request.Context(), client, c.Param("key"), evaluationContextFrom(c.Request.URL.Query())))
	})

	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Not found. Try GET /configs"})
	})

	return router
}

func configsFor(ctx context.Context, client *openfeature.Client, user openfeature.EvaluationContext) map[string]any {
	return map[string]any{
		"temporary-feature-flag": client.Boolean(ctx, "temporary-feature-flag", true, user),
		"permanent-kill-switch":  client.Boolean(ctx, "permanent-kill-switch", false, user),
		"integer-config":         client.Int(ctx, "integer-config", 10, user),
		"day-of-the-week-config": client.String(ctx, "day-of-the-week-config", "Friday", user),
		"json-value-config":      client.Object(ctx, "json-value-config", map[string]any{}, user),
	}
}

type booleanDetails struct {
	Value        bool                  `json:"value"`
	Variant      string                `json:"variant"`
	Reason       openfeature.Reason    `json:"reason"`
	ErrorCode    openfeature.ErrorCode `json:"errorCode"`
	ErrorMessage string                `json:"errorMessage"`
}

func booleanDetailsFor(ctx context.Context, client *openfeature.Client, key string, user openfeature.EvaluationContext) booleanDetails {
	details, _ := client.BooleanValueDetails(ctx, key, false, user)
	return booleanDetails{
		Value:        details.Value,
		Variant:      details.Variant,
		Reason:       details.Reason,
		ErrorCode:    details.ErrorCode,
		ErrorMessage: details.ErrorMessage,
	}
}

func evaluationContextFrom(query url.Values) openfeature.EvaluationContext {
	traits := map[string]any{}
	for parameter := range query {
		if !describesTheUser(parameter) {
			traits[parameter] = query.Get(parameter)
		}
	}

	attributes := map[string]any{
		"anonymous": query.Get("anonymous") == "true",
		"traits":    traits,
	}
	if query.Has("name") {
		attributes["name"] = query.Get("name")
	}

	return openfeature.NewEvaluationContext(query.Get("id"), attributes)
}

func describesTheUser(parameter string) bool {
	switch parameter {
	case "id", "name", "anonymous":
		return true
	default:
		return false
	}
}
