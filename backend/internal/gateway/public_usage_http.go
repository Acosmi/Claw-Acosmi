package gateway

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

type PublicUsageStatsHandlerConfig struct {
	GetMethodContext func() *GatewayMethodContext
	CORSOrigin       string
	CacheControl     string
}

var publicUsageRegistry = func() *MethodRegistry {
	registry := NewMethodRegistry()
	registry.RegisterAll(UsageHandlers())
	return registry
}()

func RegisterPublicUsageStatsRoutes(mux *http.ServeMux, cfg PublicUsageStatsHandlerConfig) {
	if mux == nil || cfg.GetMethodContext == nil {
		return
	}

	mux.HandleFunc("/public/stats/overview", publicUsageRoute(cfg, servePublicStatsOverview))
	mux.HandleFunc("/public/stats/models", publicUsageRoute(cfg, servePublicStatsModels))
	mux.HandleFunc("/public/stats/providers", publicUsageRoute(cfg, servePublicStatsProviders))
	mux.HandleFunc("/public/stats/timeseries", publicUsageRoute(cfg, servePublicStatsTimeseries))
	mux.HandleFunc("/public/stats/methodology", publicUsageRoute(cfg, servePublicStatsMethodology))
}

func publicUsageRoute(
	cfg PublicUsageStatsHandlerConfig,
	handler func(http.ResponseWriter, *http.Request, *GatewayMethodContext),
) http.HandlerFunc {
	allowOrigin := strings.TrimSpace(cfg.CORSOrigin)
	if allowOrigin == "" {
		allowOrigin = "*"
	}
	cacheControl := strings.TrimSpace(cfg.CacheControl)
	if cacheControl == "" {
		cacheControl = "public, max-age=60"
	}

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Max-Age", "86400")
		w.Header().Set("Cache-Control", cacheControl)
		w.Header().Set("X-Content-Type-Options", "nosniff")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet {
			SendMethodNotAllowed(w, "GET, OPTIONS")
			return
		}

		methodCtx := cfg.GetMethodContext()
		if methodCtx == nil {
			SendJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
				"error": map[string]interface{}{
					"code":    ErrCodeInternalError,
					"message": "public stats context is unavailable",
				},
			})
			return
		}
		handler(w, r, methodCtx)
	}
}

func servePublicStatsOverview(w http.ResponseWriter, r *http.Request, methodCtx *GatewayMethodContext) {
	usageResult, err := invokePublicUsageMethod("sessions.usage", publicStatsUsageParams(r), methodCtx)
	if err != nil {
		sendPublicUsageError(w, err)
		return
	}

	result, ok := usageResult.(map[string]interface{})
	if !ok {
		sendPublicUsageError(w, NewErrorShape(ErrCodeInternalError, "invalid usage overview payload"))
		return
	}

	totals, _ := result["totals"].(*usageTotals)
	sessions, _ := result["sessions"].([]map[string]interface{})
	aggregates, _ := result["aggregates"].(map[string]interface{})
	byModel, _ := aggregates["byModel"].([]map[string]interface{})
	byProvider, _ := aggregates["byProvider"].([]map[string]interface{})

	sourceCounts := map[string]int{
		"actual":    0,
		"estimated": 0,
		"mixed":     0,
	}
	for _, sessionEntry := range sessions {
		usage, _ := sessionEntry["usage"].(*sessionCostSummary)
		if usage == nil {
			continue
		}
		switch usage.UsageSource {
		case "actual", "estimated", "mixed":
			sourceCounts[usage.UsageSource]++
		}
	}

	payload := map[string]interface{}{
		"version":     "v1",
		"generatedAt": time.Now().UnixMilli(),
		"range":       publicStatsRange(result),
		"totals":      totals,
		"sessions": map[string]interface{}{
			"count":        len(sessions),
			"usageSources": sourceCounts,
		},
		"models": map[string]interface{}{
			"count": len(byModel),
			"top":   firstPublicStatsItem(byModel),
		},
		"providers": map[string]interface{}{
			"count": len(byProvider),
			"top":   firstPublicStatsItem(byProvider),
		},
	}
	SendJSON(w, http.StatusOK, payload)
}

func servePublicStatsModels(w http.ResponseWriter, r *http.Request, methodCtx *GatewayMethodContext) {
	usageResult, err := invokePublicUsageMethod("sessions.usage", publicStatsUsageParams(r), methodCtx)
	if err != nil {
		sendPublicUsageError(w, err)
		return
	}

	result, ok := usageResult.(map[string]interface{})
	if !ok {
		sendPublicUsageError(w, NewErrorShape(ErrCodeInternalError, "invalid usage models payload"))
		return
	}
	aggregates, _ := result["aggregates"].(map[string]interface{})
	byModel, _ := aggregates["byModel"].([]map[string]interface{})

	limit, limitErr := publicStatsLimit(r, 20, 100)
	if limitErr != nil {
		SendInvalidRequest(w, limitErr.Message)
		return
	}
	items := trimPublicStatsItems(byModel, limit)

	SendJSON(w, http.StatusOK, map[string]interface{}{
		"version":     "v1",
		"generatedAt": time.Now().UnixMilli(),
		"range":       publicStatsRange(result),
		"count":       len(byModel),
		"items":       items,
	})
}

func servePublicStatsProviders(w http.ResponseWriter, r *http.Request, methodCtx *GatewayMethodContext) {
	usageResult, err := invokePublicUsageMethod("sessions.usage", publicStatsUsageParams(r), methodCtx)
	if err != nil {
		sendPublicUsageError(w, err)
		return
	}

	result, ok := usageResult.(map[string]interface{})
	if !ok {
		sendPublicUsageError(w, NewErrorShape(ErrCodeInternalError, "invalid usage providers payload"))
		return
	}
	aggregates, _ := result["aggregates"].(map[string]interface{})
	byProvider, _ := aggregates["byProvider"].([]map[string]interface{})

	limit, limitErr := publicStatsLimit(r, 20, 100)
	if limitErr != nil {
		SendInvalidRequest(w, limitErr.Message)
		return
	}
	items := trimPublicStatsItems(byProvider, limit)

	SendJSON(w, http.StatusOK, map[string]interface{}{
		"version":     "v1",
		"generatedAt": time.Now().UnixMilli(),
		"range":       publicStatsRange(result),
		"count":       len(byProvider),
		"items":       items,
	})
}

func servePublicStatsTimeseries(w http.ResponseWriter, r *http.Request, methodCtx *GatewayMethodContext) {
	costResult, err := invokePublicUsageMethod("usage.cost", publicStatsDateParams(r), methodCtx)
	if err != nil {
		sendPublicUsageError(w, err)
		return
	}

	summary, ok := costResult.(*usageCostSummary)
	if !ok {
		sendPublicUsageError(w, NewErrorShape(ErrCodeInternalError, "invalid usage timeseries payload"))
		return
	}

	rangePayload := map[string]interface{}{
		"startDate": r.URL.Query().Get("startDate"),
		"endDate":   r.URL.Query().Get("endDate"),
		"days":      summary.Days,
	}
	if rangePayload["startDate"] == "" {
		rangePayload["startDate"] = ""
	}
	if rangePayload["endDate"] == "" {
		rangePayload["endDate"] = ""
	}
	if len(summary.Daily) > 0 {
		if rangePayload["startDate"] == "" {
			rangePayload["startDate"] = summary.Daily[0].Date
		}
		if rangePayload["endDate"] == "" {
			rangePayload["endDate"] = summary.Daily[len(summary.Daily)-1].Date
		}
	}

	SendJSON(w, http.StatusOK, map[string]interface{}{
		"version":     "v1",
		"generatedAt": time.Now().UnixMilli(),
		"range":       rangePayload,
		"granularity": "day",
		"items":       summary.Daily,
		"totals":      summary.Totals,
	})
}

func servePublicStatsMethodology(w http.ResponseWriter, _ *http.Request, _ *GatewayMethodContext) {
	SendJSON(w, http.StatusOK, map[string]interface{}{
		"version":     "v1",
		"generatedAt": time.Now().UnixMilli(),
		"privacy": map[string]interface{}{
			"included": []string{
				"aggregate model usage totals",
				"aggregate provider usage totals",
				"aggregate daily token and cost rollups",
				"aggregate session counts by usage source",
			},
			"excluded": []string{
				"user identifiers",
				"session identifiers",
				"channel identifiers",
				"raw prompts and message logs",
			},
		},
		"definitions": map[string]interface{}{
			"actualUsage":    "Tokens and cost recorded directly in session transcripts.",
			"estimatedUsage": "Fallback token estimates derived from transcript text when recorded usage is missing.",
			"totalTokens":    "Input + output + cache read + cache write tokens.",
		},
		"endpoints": []map[string]string{
			{"path": "/public/stats/overview", "purpose": "Public summary cards and toplines."},
			{"path": "/public/stats/models", "purpose": "Aggregated usage by model."},
			{"path": "/public/stats/providers", "purpose": "Aggregated usage by provider."},
			{"path": "/public/stats/timeseries", "purpose": "Daily aggregated usage totals."},
			{"path": "/public/stats/methodology", "purpose": "Public data and privacy methodology."},
		},
		"query": map[string]interface{}{
			"startDate": "Optional YYYY-MM-DD inclusive lower bound.",
			"endDate":   "Optional YYYY-MM-DD inclusive upper bound.",
			"limit":     "Optional item limit for models/providers endpoints.",
		},
	})
}

func invokePublicUsageMethod(method string, params map[string]interface{}, methodCtx *GatewayMethodContext) (interface{}, *ErrorShape) {
	var (
		gotOK      bool
		gotPayload interface{}
		gotErr     *ErrorShape
	)

	HandleGatewayRequest(publicUsageRegistry, &RequestFrame{
		Method: method,
		Params: params,
	}, nil, methodCtx, func(ok bool, payload interface{}, err *ErrorShape) {
		gotOK = ok
		gotPayload = payload
		gotErr = err
	})

	if !gotOK {
		if gotErr == nil {
			gotErr = NewErrorShape(ErrCodeInternalError, "public usage method failed")
		}
		return nil, gotErr
	}
	return gotPayload, nil
}

func publicStatsDateParams(r *http.Request) map[string]interface{} {
	params := map[string]interface{}{}
	if startDate := strings.TrimSpace(r.URL.Query().Get("startDate")); startDate != "" {
		params["startDate"] = startDate
	}
	if endDate := strings.TrimSpace(r.URL.Query().Get("endDate")); endDate != "" {
		params["endDate"] = endDate
	}
	return params
}

func publicStatsUsageParams(r *http.Request) map[string]interface{} {
	params := publicStatsDateParams(r)
	params["includeContextWeight"] = false
	params["limit"] = float64(5000)
	return params
}

func publicStatsLimit(r *http.Request, defaultValue, maxValue int) (int, *ErrorShape) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return defaultValue, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, NewErrorShape(ErrCodeBadRequest, "limit must be a positive integer")
	}
	if limit > maxValue {
		limit = maxValue
	}
	return limit, nil
}

func publicStatsRange(result map[string]interface{}) map[string]interface{} {
	rangePayload := map[string]interface{}{
		"startDate": result["startDate"],
		"endDate":   result["endDate"],
	}
	if daily, ok := result["daily"].([]map[string]interface{}); ok {
		rangePayload["days"] = len(daily)
	}
	return rangePayload
}

func firstPublicStatsItem(items []map[string]interface{}) interface{} {
	if len(items) == 0 {
		return nil
	}
	return items[0]
}

func trimPublicStatsItems(items []map[string]interface{}, limit int) []map[string]interface{} {
	if len(items) == 0 {
		return []map[string]interface{}{}
	}
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

func sendPublicUsageError(w http.ResponseWriter, err *ErrorShape) {
	status := http.StatusInternalServerError
	if err != nil && err.Code == ErrCodeBadRequest {
		status = http.StatusBadRequest
	}
	if err == nil {
		err = NewErrorShape(ErrCodeInternalError, "public usage endpoint failed")
	}
	SendJSON(w, status, map[string]interface{}{
		"error": map[string]interface{}{
			"code":    err.Code,
			"message": err.Message,
		},
	})
}
