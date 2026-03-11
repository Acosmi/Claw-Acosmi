package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicUsageStatsOverview(t *testing.T) {
	fixture := newUsageHandlersFixture(t)
	mux := http.NewServeMux()
	RegisterPublicUsageStatsRoutes(mux, PublicUsageStatsHandlerConfig{
		GetMethodContext: func() *GatewayMethodContext { return fixture.context },
	})

	req := httptest.NewRequest(http.MethodGet, "/public/stats/overview?startDate=2025-01-01&endDate=2025-01-31", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want *", got)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode overview response: %v", err)
	}

	if payload["version"] != "v1" {
		t.Fatalf("version = %v, want v1", payload["version"])
	}
	rangePayload, _ := payload["range"].(map[string]interface{})
	if rangePayload["startDate"] != "2025-01-01" || rangePayload["endDate"] != "2025-01-31" {
		t.Fatalf("unexpected range payload: %+v", rangePayload)
	}
	totals, _ := payload["totals"].(map[string]interface{})
	if int(totals["totalTokens"].(float64)) != fixture.expectedActualTokens+fixture.estimatedUserTokens {
		t.Fatalf("unexpected totals payload: %+v", totals)
	}
	sessions, _ := payload["sessions"].(map[string]interface{})
	if int(sessions["count"].(float64)) != 1 {
		t.Fatalf("sessions.count = %v, want 1", sessions["count"])
	}
	usageSources, _ := sessions["usageSources"].(map[string]interface{})
	if int(usageSources["mixed"].(float64)) != 1 {
		t.Fatalf("usageSources.mixed = %v, want 1", usageSources["mixed"])
	}
	models, _ := payload["models"].(map[string]interface{})
	if int(models["count"].(float64)) != 1 {
		t.Fatalf("models.count = %v, want 1", models["count"])
	}
}

func TestPublicUsageStatsModelsAndProviders(t *testing.T) {
	fixture := newUsageHandlersFixture(t)
	mux := http.NewServeMux()
	RegisterPublicUsageStatsRoutes(mux, PublicUsageStatsHandlerConfig{
		GetMethodContext: func() *GatewayMethodContext { return fixture.context },
	})

	t.Run("models", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/public/stats/models?startDate=2025-01-01&endDate=2025-01-31&limit=1", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status code = %d, want 200", rec.Code)
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode models response: %v", err)
		}

		items, _ := payload["items"].([]interface{})
		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		first, _ := items[0].(map[string]interface{})
		if first["provider"] != "openai" || first["model"] != "gpt-5.1" {
			t.Fatalf("unexpected first model item: %+v", first)
		}
		if _, exists := first["sessionId"]; exists {
			t.Fatalf("public model item leaked sessionId: %+v", first)
		}
	})

	t.Run("providers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/public/stats/providers?startDate=2025-01-01&endDate=2025-01-31&limit=1", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status code = %d, want 200", rec.Code)
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode providers response: %v", err)
		}

		items, _ := payload["items"].([]interface{})
		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		first, _ := items[0].(map[string]interface{})
		if first["provider"] != "openai" {
			t.Fatalf("unexpected first provider item: %+v", first)
		}
	})
}

func TestPublicUsageStatsTimeseriesAndMethodology(t *testing.T) {
	fixture := newUsageHandlersFixture(t)
	mux := http.NewServeMux()
	RegisterPublicUsageStatsRoutes(mux, PublicUsageStatsHandlerConfig{
		GetMethodContext: func() *GatewayMethodContext { return fixture.context },
	})

	t.Run("timeseries", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/public/stats/timeseries?startDate=2025-01-01&endDate=2025-01-31", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status code = %d, want 200", rec.Code)
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode timeseries response: %v", err)
		}

		if payload["granularity"] != "day" {
			t.Fatalf("granularity = %v, want day", payload["granularity"])
		}
		items, _ := payload["items"].([]interface{})
		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		first, _ := items[0].(map[string]interface{})
		if int(first["totalTokens"].(float64)) != fixture.expectedActualTokens+fixture.estimatedUserTokens {
			t.Fatalf("unexpected timeseries item: %+v", first)
		}
	})

	t.Run("methodology", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/public/stats/methodology", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status code = %d, want 200", rec.Code)
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode methodology response: %v", err)
		}

		endpoints, _ := payload["endpoints"].([]interface{})
		if len(endpoints) != 5 {
			t.Fatalf("len(endpoints) = %d, want 5", len(endpoints))
		}
	})
}

func TestPublicUsageStatsModelsRejectInvalidLimit(t *testing.T) {
	fixture := newUsageHandlersFixture(t)
	mux := http.NewServeMux()
	RegisterPublicUsageStatsRoutes(mux, PublicUsageStatsHandlerConfig{
		GetMethodContext: func() *GatewayMethodContext { return fixture.context },
	})

	req := httptest.NewRequest(http.MethodGet, "/public/stats/models?limit=bad", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want 400", rec.Code)
	}
}
