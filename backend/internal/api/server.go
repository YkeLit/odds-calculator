package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"odds-calculator/backend/internal/auth"
	"odds-calculator/backend/internal/holdem"
	"odds-calculator/backend/internal/mahjong"
	"odds-calculator/backend/internal/models"
	"odds-calculator/backend/internal/storage"
)

type Server struct {
	authService *auth.Service
	store       *storage.Store
}

func NewServer(authService *auth.Service, store *storage.Store) *Server {
	return &Server{authService: authService, store: store}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/register", s.handleRegister)
	mux.HandleFunc("/api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("/api/v1/holdem/odds", s.handleHoldemOdds)
	mux.HandleFunc("/api/v1/holdem/allin-ev", s.handleHoldemAllInEV)
	mux.HandleFunc("/api/v1/holdem/decision", s.handleHoldemDecision)
	mux.HandleFunc("/api/v1/mahjong/analyze", s.handleMahjongAnalyze)
	mux.HandleFunc("/api/v1/history", s.handleHistory)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return cors(mux)
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req models.RegisterRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.authService.Register(req)
	if err != nil {
		if errors.Is(err, auth.ErrUserExists) {
			writeError(w, http.StatusConflict, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req models.LoginRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := s.authService.Login(req)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleHoldemOdds(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req models.HoldemOddsRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := holdem.CalculateOdds(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if user, ok := s.optionalUser(r); ok {
		_ = s.store.CreateHistory(user.ID, "holdem_odds", req, resp)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleHoldemAllInEV(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req models.HoldemAllInEVRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := holdem.CalculateAllInEV(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if user, ok := s.optionalUser(r); ok {
		_ = s.store.CreateHistory(user.ID, "holdem_allin_ev", req, resp)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleHoldemDecision(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req models.HoldemDecisionRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := holdem.CalculateDecision(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if user, ok := s.optionalUser(r); ok {
		_ = s.store.CreateHistory(user.ID, "holdem_decision", req, resp)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleMahjongAnalyze(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req models.MahjongAnalyzeRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := mahjong.Analyze(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if user, ok := s.optionalUser(r); ok {
		_ = s.store.CreateHistory(user.ID, "mahjong", req, resp)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	user, err := s.requireUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	page := parseInt(r.URL.Query().Get("page"), 1)
	pageSize := parseInt(r.URL.Query().Get("pageSize"), 20)
	gameType := r.URL.Query().Get("gameType")
	items, total, err := s.store.ListHistory(user.ID, gameType, page, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, models.HistoryResponse{
		Items:    items,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	})
}

func (s *Server) requireUser(r *http.Request) (models.UserInfo, error) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		return models.UserInfo{}, auth.ErrInvalidToken
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return models.UserInfo{}, auth.ErrInvalidToken
	}
	return s.authService.ParseToken(parts[1])
}

func (s *Server) optionalUser(r *http.Request) (models.UserInfo, bool) {
	user, err := s.requireUser(r)
	if err != nil {
		return models.UserInfo{}, false
	}
	return user, true
}

func parseInt(raw string, def int) int {
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	if r.Method != method {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return false
	}
	return true
}

func readJSON(r *http.Request, dst any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	return nil
}

func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
