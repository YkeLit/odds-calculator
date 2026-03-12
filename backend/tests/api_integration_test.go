package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"odds-calculator/backend/internal/api"
	"odds-calculator/backend/internal/auth"
	"odds-calculator/backend/internal/models"
	"odds-calculator/backend/internal/storage"
)

func TestAuthAndHistoryFlow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "integration.db")
	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	defer store.Close()

	authSvc := auth.NewService(store, "integration-secret", time.Hour)
	h := api.NewServer(authSvc, store)
	handler := h.Routes()

	registerResp := models.AuthResponse{}
	if code, err := doJSON(handler, "/api/v1/auth/register", "POST", models.RegisterRequest{
		Username: "alice",
		Password: "password123",
	}, "", &registerResp); err != nil || code != http.StatusCreated {
		t.Fatalf("register failed code=%d err=%v", code, err)
	}

	loginResp := models.AuthResponse{}
	if code, err := doJSON(handler, "/api/v1/auth/login", "POST", models.LoginRequest{
		Username: "alice",
		Password: "password123",
	}, "", &loginResp); err != nil || code != http.StatusOK {
		t.Fatalf("login failed code=%d err=%v", code, err)
	}
	if loginResp.AccessToken == "" {
		t.Fatalf("expected access token")
	}

	oddsResp := models.HoldemOddsResponse{}
	if code, err := doJSON(handler, "/api/v1/holdem/odds", "POST", models.HoldemOddsRequest{
		Players: []models.PlayerInput{
			{ID: "p1", HoleCards: []string{"As", "Ah"}},
			{ID: "p2", HoleCards: []string{"Kc", "Kd"}},
		},
		BoardCards: []string{"2c", "7d", "9h", "Js", "3d"},
	}, "Bearer "+loginResp.AccessToken, &oddsResp); err != nil || code != http.StatusOK {
		t.Fatalf("odds failed code=%d err=%v", code, err)
	}

	historyResp := models.HistoryResponse{}
	if code, err := doJSON(handler, "/api/v1/history?page=1&pageSize=10", "GET", nil, "Bearer "+loginResp.AccessToken, &historyResp); err != nil || code != http.StatusOK {
		t.Fatalf("history failed code=%d err=%v", code, err)
	}
	if historyResp.Total < 1 || len(historyResp.Items) < 1 {
		t.Fatalf("expected at least one history item, got %+v", historyResp)
	}

	// Test decision API
	decisionResp := models.HoldemDecisionResponse{}
	decisionReq := models.HoldemDecisionRequest{
		Hero: models.HeroState{
			HoleCards: []string{"As", "Ah"},
			Position:  "BTN",
			Stack:     100,
		},
		Table: models.TableState{
			PlayerCount: 2,
			Positions: []string{"BB", "BTN"},
			EffectiveStacks: map[string]float64{"BB": 100, "BTN": 100},
		},
		Street: models.StreetPreflop,
		BoardCards: []string{},
		PotState: models.PotState{
			PotSize: 3,
			ToCall: 1,
			MinRaiseTo: 4,
			Blinds: [2]float64{1, 2},
		},
		ActionHistory: []models.ActionNode{},
		Opponents: []models.OpponentInfo{
			{ID: "BB", Position: "BB"},
		},
		SolverConfig: models.SolverConfig{BranchCount: 2, TimeoutMs: 1000, RolloutBudget: 100},
	}
	if code, err := doJSON(handler, "/api/v1/holdem/decision", "POST", decisionReq, "Bearer "+loginResp.AccessToken, &decisionResp); err != nil || code != http.StatusOK {
		t.Fatalf("decision failed code=%d err=%v", code, err)
	}

	historyResp2 := models.HistoryResponse{}
	if code, err := doJSON(handler, "/api/v1/history?page=1&pageSize=10", "GET", nil, "Bearer "+loginResp.AccessToken, &historyResp2); err != nil || code != http.StatusOK {
		t.Fatalf("history failed code=%d err=%v", code, err)
	}
	if historyResp2.Total != historyResp.Total + 1 {
		t.Fatalf("expected history total to increment by 1 after decision API, got %d", historyResp2.Total)
	}
}

func TestHistoryRequiresAuth(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "integration.db")
	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	defer store.Close()

	authSvc := auth.NewService(store, "integration-secret", time.Hour)
	h := api.NewServer(authSvc, store)
	handler := h.Routes()

	code, err := doJSON(handler, "/api/v1/history", "GET", nil, "", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", code)
	}
}

func doJSON(handler http.Handler, path, method string, body any, authHeader string, out any) (int, error) {
	var payload []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		payload = encoded
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	resp := rr.Result()
	defer resp.Body.Close()

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}
