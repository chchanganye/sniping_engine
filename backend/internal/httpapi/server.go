package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	"sniping_engine/internal/config"
	"sniping_engine/internal/engine"
	"sniping_engine/internal/logbus"
	"sniping_engine/internal/model"
	"sniping_engine/internal/notify"
	"sniping_engine/internal/store/sqlite"
	"sniping_engine/internal/utils"
	"sniping_engine/internal/ws"
)

const defaultTenantID = "1"

type Options struct {
	Cfg      config.Config
	Bus      *logbus.Bus
	Store    *sqlite.Store
	Engine   *engine.Engine
	Notifier notify.Notifier
}

type Server struct {
	cfg          config.Config
	bus          *logbus.Bus
	store        *sqlite.Store
	engine       *engine.Engine
	notif        notify.Notifier
	ws           *ws.Handler
	anonSessions *anonSessionStore
}

func New(opts Options) *Server {
	return &Server{
		cfg:          opts.Cfg,
		bus:          opts.Bus,
		store:        opts.Store,
		engine:       opts.Engine,
		notif:        opts.Notifier,
		ws:           ws.NewHandler(opts.Bus, opts.Cfg.Server.Cors.AllowOrigins),
		anonSessions: newAnonSessionStore(30*time.Minute, 2000),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.Handle("/ws", s.ws)

	api := http.NewServeMux()
	api.HandleFunc("/api/v1/accounts", s.handleAccounts)
	api.HandleFunc("/api/v1/targets", s.handleTargets)
	api.HandleFunc("/api/v1/engine/start", s.handleEngineStart)
	api.HandleFunc("/api/v1/engine/stop", s.handleEngineStop)
	api.HandleFunc("/api/v1/engine/state", s.handleEngineState)
	api.HandleFunc("/api/v1/engine/preflight", s.handleEnginePreflight)
	api.HandleFunc("/api/v1/engine/test-buy", s.handleEngineTestBuy)
	api.HandleFunc("/api/v1/captcha/state", s.handleCaptchaState)
	api.HandleFunc("/api/v1/settings/email", s.handleEmailSettings)
	api.HandleFunc("/api/v1/settings/email/test", s.handleEmailTest)
	api.HandleFunc("/api/v1/settings/limits", s.handleLimitsSettings)
	api.HandleFunc("/api/", s.handleUpstreamProxy)

	mux.Handle("/api/", corsMiddleware(s.cfg.Server.Cors, api))
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleCaptchaState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": utils.GetCaptchaEngineStatus()})
}

func (s *Server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		accounts, err := s.store.ListAccounts(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": accounts})
	case http.MethodPost:
		type accountUpsertPayload struct {
			ID          string  `json:"id,omitempty"`
			Username    *string `json:"username,omitempty"`
			Mobile      string  `json:"mobile"`
			Token       *string `json:"token,omitempty"`
			UserAgent   *string `json:"userAgent,omitempty"`
			DeviceID    *string `json:"deviceId,omitempty"`
			UUID        *string `json:"uuid,omitempty"`
			Proxy       *string `json:"proxy,omitempty"`
			AddressID   *int64  `json:"addressId,omitempty"`
			DivisionIDs *string `json:"divisionIds,omitempty"`
		}

		var body accountUpsertPayload
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		mobile := strings.TrimSpace(body.Mobile)
		if mobile == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "mobile is required"})
			return
		}

		var current model.Account
		if strings.TrimSpace(body.ID) != "" {
			if found, err := s.store.GetAccount(r.Context(), strings.TrimSpace(body.ID)); err == nil {
				current = found
			}
		}
		if strings.TrimSpace(current.ID) == "" {
			if found, err := s.store.GetAccountByMobile(r.Context(), mobile); err == nil {
				current = found
			}
		}

		next := current
		next.Mobile = mobile
		if strings.TrimSpace(body.ID) != "" {
			next.ID = strings.TrimSpace(body.ID)
		}
		if body.Username != nil {
			next.Username = strings.TrimSpace(*body.Username)
		}
		if body.UserAgent != nil {
			next.UserAgent = strings.TrimSpace(*body.UserAgent)
		}
		if body.DeviceID != nil {
			next.DeviceID = strings.TrimSpace(*body.DeviceID)
		}
		if body.UUID != nil {
			next.UUID = strings.TrimSpace(*body.UUID)
		}
		if body.Proxy != nil {
			next.Proxy = strings.TrimSpace(*body.Proxy)
		}
		if body.AddressID != nil {
			next.AddressID = *body.AddressID
		}
		if body.DivisionIDs != nil {
			next.DivisionIDs = strings.TrimSpace(*body.DivisionIDs)
		}
		if body.Token != nil {
			t := strings.TrimSpace(*body.Token)
			next.Token = t
			if t == "" {
				next.Cookies = nil
			}
		}

		acc, err := s.store.UpsertAccount(r.Context(), next)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": acc})
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "id is required"})
			return
		}
		if err := s.store.DeleteAccount(r.Context(), id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTargets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		targets, err := s.store.ListTargets(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": targets})
	case http.MethodPost:
		type targetUpsertPayload struct {
			ID                 string           `json:"id"`
			Name               string           `json:"name,omitempty"`
			ImageURL           string           `json:"imageUrl,omitempty"`
			ItemID             int64            `json:"itemId"`
			SKUID              int64            `json:"skuId"`
			ShopID             int64            `json:"shopId,omitempty"`
			Mode               model.TargetMode `json:"mode"`
			TargetQty          int              `json:"targetQty"`
			PerOrderQty        int              `json:"perOrderQty"`
			RushAtMs           int64            `json:"rushAtMs,omitempty"`
			RushLeadMs         *int64           `json:"rushLeadMs,omitempty"`
			CaptchaVerifyParam *string          `json:"captchaVerifyParam,omitempty"`
			Enabled            bool             `json:"enabled"`
		}

		var body targetUpsertPayload
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}

		next := model.Target{
			ID:          strings.TrimSpace(body.ID),
			Name:        strings.TrimSpace(body.Name),
			ImageURL:    strings.TrimSpace(body.ImageURL),
			ItemID:      body.ItemID,
			SKUID:       body.SKUID,
			ShopID:      body.ShopID,
			Mode:        body.Mode,
			TargetQty:   body.TargetQty,
			PerOrderQty: body.PerOrderQty,
			RushAtMs:    body.RushAtMs,
			Enabled:     body.Enabled,
		}
		if body.RushLeadMs != nil {
			next.RushLeadMs = *body.RushLeadMs
		} else if next.ID != "" {
			if current, err := s.store.GetTarget(r.Context(), next.ID); err == nil {
				next.RushLeadMs = current.RushLeadMs
			}
		}
		if body.CaptchaVerifyParam != nil {
			next.CaptchaVerifyParam = strings.TrimSpace(*body.CaptchaVerifyParam)
		} else if next.ID != "" {
			if current, err := s.store.GetTarget(r.Context(), next.ID); err == nil {
				next.CaptchaVerifyParam = current.CaptchaVerifyParam
			}
		}

		t, err := s.store.UpsertTarget(r.Context(), next)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": t})
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "id is required"})
			return
		}
		if err := s.store.DeleteTarget(r.Context(), id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleEngineStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.engine.StartAll(ctx); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleEngineStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.engine.StopAll(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleEngineState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": s.engine.State()})
}

type enginePreflightPayload struct {
	TargetID string `json:"targetId"`
}

func (s *Server) handleEnginePreflight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.engine == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "engine unavailable"})
		return
	}
	var body enginePreflightPayload
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if strings.TrimSpace(body.TargetID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "targetId is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	res, err := s.engine.PreflightOnce(ctx, strings.TrimSpace(body.TargetID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": res})
}

type engineTestBuyPayload struct {
	TargetID           string `json:"targetId"`
	CaptchaVerifyParam string `json:"captchaVerifyParam,omitempty"`
	OpID               string `json:"opId,omitempty"`
}

func (s *Server) handleEngineTestBuy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.engine == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "engine unavailable"})
		return
	}
	var body engineTestBuyPayload
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if strings.TrimSpace(body.TargetID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "targetId is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	res, err := s.engine.TestBuyOnce(ctx, strings.TrimSpace(body.TargetID), strings.TrimSpace(body.CaptchaVerifyParam), strings.TrimSpace(body.OpID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": res})
}

type emailSettingsPayload struct {
	Enabled  *bool   `json:"enabled,omitempty"`
	Email    *string `json:"email,omitempty"`
	AuthCode *string `json:"authCode,omitempty"`
}

func (s *Server) handleEmailSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		val, ok, err := s.store.GetEmailSettings(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		if !ok {
			writeJSON(w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"enabled":  false,
					"email":    "",
					"authCode": "",
				},
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": val})
	case http.MethodPost:
		var body emailSettingsPayload
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}

		current, _, err := s.store.GetEmailSettings(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}

		next := current
		if body.Enabled != nil {
			next.Enabled = *body.Enabled
		}
		if body.Email != nil {
			next.Email = strings.TrimSpace(*body.Email)
		}
		if body.AuthCode != nil {
			ac := strings.TrimSpace(*body.AuthCode)
			if ac != "******" {
				next.AuthCode = ac
			}
		}

		saved, err := s.store.UpsertEmailSettings(r.Context(), next)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": saved})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type emailTestPayload struct {
	Email    string `json:"email,omitempty"`
	AuthCode string `json:"authCode,omitempty"`
}

type limitsSettingsPayload struct {
	MaxPerTargetInFlight *int `json:"maxPerTargetInFlight,omitempty"`
	CaptchaMaxInFlight   *int `json:"captchaMaxInFlight,omitempty"`
}

func (s *Server) handleLimitsSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		val, ok, err := s.store.GetLimitsSettings(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		if !ok {
			maxPerTarget := s.cfg.Limits.MaxPerTargetInFlight
			if maxPerTarget <= 0 {
				maxPerTarget = 1
			}
			captchaMax := s.cfg.Limits.CaptchaMaxInFlight
			if captchaMax <= 0 {
				captchaMax = 1
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"data": model.LimitsSettings{
					MaxPerTargetInFlight: maxPerTarget,
					CaptchaMaxInFlight:   captchaMax,
				},
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": val})
	case http.MethodPost:
		var body limitsSettingsPayload
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}

		current, ok, err := s.store.GetLimitsSettings(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		if !ok {
			current.MaxPerTargetInFlight = s.cfg.Limits.MaxPerTargetInFlight
			current.CaptchaMaxInFlight = s.cfg.Limits.CaptchaMaxInFlight
		}

		next := current
		if body.MaxPerTargetInFlight != nil {
			next.MaxPerTargetInFlight = *body.MaxPerTargetInFlight
		}
		if body.CaptchaMaxInFlight != nil {
			next.CaptchaMaxInFlight = *body.CaptchaMaxInFlight
		}

		if next.MaxPerTargetInFlight <= 0 {
			next.MaxPerTargetInFlight = 1
		}
		if next.CaptchaMaxInFlight <= 0 {
			next.CaptchaMaxInFlight = 1
		}
		if next.MaxPerTargetInFlight > 200 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "maxPerTargetInFlight is too large"})
			return
		}
		if next.CaptchaMaxInFlight > 50 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "captchaMaxInFlight is too large"})
			return
		}

		saved, err := s.store.UpsertLimitsSettings(r.Context(), next)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}

		if s.engine != nil {
			s.engine.SetMaxPerTargetInFlight(saved.MaxPerTargetInFlight)
		}
		utils.SetCaptchaMaxConcurrent(saved.CaptchaMaxInFlight)

		writeJSON(w, http.StatusOK, map[string]any{"data": saved})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleEmailTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body emailTestPayload
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	val, _, err := s.store.GetEmailSettings(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if strings.TrimSpace(body.Email) != "" {
		val.Email = strings.TrimSpace(body.Email)
	}
	if strings.TrimSpace(body.AuthCode) != "" {
		val.AuthCode = strings.TrimSpace(body.AuthCode)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	if err := notify.SendOrderCreatedEmail(ctx, val, notify.OrderCreatedEvent{
		At:         time.Now().UnixMilli(),
		AccountID:  "test",
		Mobile:     "test",
		TargetID:   "test",
		TargetName: "邮件测试：招财纳福牌",
		Mode:       "rush",
		ItemID:     110005201029005,
		SKUID:      110005201029005,
		ShopID:     1100078037,
		Quantity:   1,
		OrderID:    "TEST-ORDER-" + strconv.FormatInt(time.Now().Unix(), 10),
		TraceID:    "test-trace",
	}); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func parseInt(v string, def int) (int, error) {
	if strings.TrimSpace(v) == "" {
		return def, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, err
	}
	return n, nil
}

func parseInt64(v string) (int64, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func parseFloat64(v string) (float64, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return 0, err
	}
	return f, nil
}

func parseBool(v string, def bool) (bool, error) {
	if strings.TrimSpace(v) == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return false, err
	}
	return b, nil
}

func parseBoolishInt(v string, def int) (int, error) {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return def, nil
	}
	switch strings.ToLower(trimmed) {
	case "true":
		return 1, nil
	case "false":
		return 0, nil
	}
	n, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (s *Server) handleUpstreamProxy(w http.ResponseWriter, r *http.Request) {
	// Never forward internal endpoints.
	if strings.HasPrefix(r.URL.Path, "/api/v1/") {
		http.NotFound(w, r)
		return
	}
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "store unavailable"})
		return
	}
	if strings.TrimSpace(s.cfg.Provider.BaseURL) == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "provider.baseURL not configured"})
		return
	}

	upURL, err := buildUpstreamURL(s.cfg.Provider.BaseURL, r.URL.Path, r.URL.RawQuery)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
	}

	if r.Method == http.MethodPost && r.URL.Path == "/api/user/web/login/identify" && len(body) > 0 {
		body = transformIdentifyLoginBody(body)
	}

	token := extractToken(r)

	var (
		acc        model.Account
		client     *resty.Client
		jar        *cookiejar.Jar
		baseURL    *url.URL
		persistAcc bool
	)

	if token != "" {
		found, err := s.store.GetAccountByToken(r.Context(), token)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "account not found for token"})
			return
		}
		found.Token = token
		acc = found
		persistAcc = true

		c, j, b, err := s.newUpstreamClient(acc)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		client, jar, baseURL = c, j, b
	} else {
		if !isAnonymousAllowedPath(r.URL.Path) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing token (Authorization/token/x-token)"})
			return
		}
		if s.anonSessions == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "anonymous session store unavailable"})
			return
		}
		j, err := s.anonSessions.GetOrCreate(w, r)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		c, b, err := s.newAnonymousUpstreamClient(j, strings.TrimSpace(r.Header.Get("User-Agent")))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		client, jar, baseURL = c, j, b
	}

	req := client.R().SetContext(r.Context())
	if ct := strings.TrimSpace(r.Header.Get("Content-Type")); ct != "" {
		req.SetHeader("Content-Type", ct)
	}
	if accept := strings.TrimSpace(r.Header.Get("Accept")); accept != "" {
		req.SetHeader("Accept", accept)
	}
	if lang := strings.TrimSpace(r.Header.Get("Accept-Language")); lang != "" {
		req.SetHeader("Accept-Language", lang)
	}
	if len(body) > 0 {
		req.SetBody(body)
	}

	resp, err := req.Execute(r.Method, upURL.String())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}

	if persistAcc {
		if r.URL.Path == "/api/user/web/current-user" {
			if username := extractCurrentUserUsername(resp.Body()); username != "" {
				acc.Username = username
			}
		}
		acc.Cookies = exportCookies(baseURL, jar)
		_, _ = s.store.UpsertAccount(r.Context(), acc)
	}
	if token == "" && (r.URL.Path == "/api/user/web/login/login-by-sms-code" || r.URL.Path == "/api/user/web/login/identify") {
		_ = s.tryPersistLoginSession(r.Context(), body, resp.Body(), baseURL, jar)
	}

	if ct := strings.TrimSpace(resp.Header().Get("Content-Type")); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(resp.StatusCode())
	_, _ = w.Write(resp.Body())
}

func isAnonymousAllowedPath(path string) bool {
	switch path {
	case "/api/user/web/get-captcha",
		"/api/user/web/login/login-send-sms-code",
		"/api/user/web/login/login-by-sms-code",
		"/api/user/web/login/identify":
		return true
	default:
		return false
	}
}

func transformIdentifyLoginBody(body []byte) []byte {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil || m == nil {
		return body
	}

	pwd, _ := m["password"].(string)
	pwd = strings.TrimSpace(pwd)
	if pwd != "" {
		// 如果前端传的是明文，这里统一转成加密串；如果已经是 ==xxx==，就不二次加密
		if !(strings.HasPrefix(pwd, "==") && strings.HasSuffix(pwd, "==")) {
			m["password"] = utils.EncryptPayload(pwd)
		}
	}

	// 兜底补齐关键字段，避免前端漏传导致登录失败
	if _, ok := m["isApp"]; !ok {
		m["isApp"] = true
	}
	if v, ok := m["deviceType"].(string); !ok || strings.TrimSpace(v) == "" {
		m["deviceType"] = "WXAPP"
	}

	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return out
}

func (s *Server) newAnonymousUpstreamClient(jar *cookiejar.Jar, userAgent string) (*resty.Client, *url.URL, error) {
	if jar == nil {
		return nil, nil, errors.New("cookie jar is required")
	}

	baseURL, err := url.Parse(strings.TrimSpace(s.cfg.Provider.BaseURL))
	if err != nil {
		return nil, nil, err
	}

	client := resty.New().
		SetTimeout(s.cfg.Provider.Timeout()).
		SetCookieJar(jar).
		SetRetryCount(s.cfg.Provider.Retry.Count).
		SetRetryWaitTime(s.cfg.Provider.Retry.Wait()).
		SetRetryMaxWaitTime(s.cfg.Provider.Retry.MaxWait()).
		AddRetryCondition(func(r *resty.Response, err error) bool {
			if err != nil {
				return true
			}
			if r == nil {
				return true
			}
			return r.StatusCode() >= 500
		})

	proxy := strings.TrimSpace(s.cfg.Proxy.Global)
	if proxy != "" {
		client.SetProxy(proxy)
	}

	ua := strings.TrimSpace(userAgent)
	if ua == "" {
		ua = strings.TrimSpace(s.cfg.Provider.UserAgent)
	}
	client.SetHeader("User-Agent", utils.NormalizeWXAppUserAgent(ua))
	client.SetHeader("device-type", "WXAPP")
	client.SetHeader("tenantId", defaultTenantID)
	client.SetHeader("x-requested-with", "XMLHttpRequest")

	client.OnBeforeRequest(func(_ *resty.Client, req *resty.Request) error {
		if s.bus != nil {
			s.bus.Log("debug", "代理请求", map[string]any{
				"method": req.Method,
				"url":    req.URL,
			})
		}
		return nil
	})

	return client, baseURL, nil
}

func (s *Server) tryPersistLoginSession(ctx context.Context, reqBody, respBody []byte, baseURL *url.URL, jar *cookiejar.Jar) error {
	if s.store == nil {
		return nil
	}
	mobile, ua, deviceID, uuid, err := extractLoginRequestFields(reqBody)
	if err != nil || strings.TrimSpace(mobile) == "" {
		return nil
	}
	token, err := extractLoginToken(respBody)
	if err != nil || strings.TrimSpace(token) == "" {
		return nil
	}

	existing, _ := s.store.GetAccountByMobile(ctx, strings.TrimSpace(mobile))
	acc := existing
	acc.Mobile = strings.TrimSpace(mobile)
	acc.Token = strings.TrimSpace(token)
	if strings.TrimSpace(acc.UserAgent) == "" && strings.TrimSpace(ua) != "" {
		acc.UserAgent = strings.TrimSpace(ua)
	}
	if strings.TrimSpace(acc.DeviceID) == "" && strings.TrimSpace(deviceID) != "" {
		acc.DeviceID = strings.TrimSpace(deviceID)
	}
	if strings.TrimSpace(acc.UUID) == "" && strings.TrimSpace(uuid) != "" {
		acc.UUID = strings.TrimSpace(uuid)
	}
	acc.Cookies = exportCookies(baseURL, jar)
	if strings.TrimSpace(acc.Username) == "" {
		if username, _ := s.fetchCurrentUserUsername(ctx, jar, token, ua); strings.TrimSpace(username) != "" {
			acc.Username = strings.TrimSpace(username)
		}
	}

	_, err = s.store.UpsertAccount(ctx, acc)
	return err
}

func extractLoginRequestFields(body []byte) (mobile string, userAgent string, deviceID string, uuid string, err error) {
	if len(body) == 0 {
		return "", "", "", "", nil
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return "", "", "", "", err
	}
	if v, ok := m["mobile"].(string); ok {
		mobile = v
	}
	if strings.TrimSpace(mobile) == "" {
		if v, ok := m["identify"].(string); ok {
			mobile = v
		}
	}
	if v, ok := m["userAgent"].(string); ok {
		userAgent = v
	}
	if v, ok := m["deviceId"].(string); ok {
		deviceID = v
	}
	if v, ok := m["uuid"].(string); ok {
		uuid = v
	}
	return mobile, userAgent, deviceID, uuid, nil
}

func (s *Server) fetchCurrentUserUsername(ctx context.Context, jar *cookiejar.Jar, token string, userAgent string) (string, error) {
	if jar == nil {
		return "", errors.New("cookie jar is required")
	}
	if strings.TrimSpace(s.cfg.Provider.BaseURL) == "" {
		return "", errors.New("provider.baseURL not configured")
	}
	if strings.TrimSpace(token) == "" {
		return "", errors.New("token is required")
	}

	u, err := buildUpstreamURL(s.cfg.Provider.BaseURL, "/api/user/web/current-user", "")
	if err != nil {
		return "", err
	}

	client := resty.New().
		SetTimeout(s.cfg.Provider.Timeout()).
		SetCookieJar(jar).
		SetRetryCount(s.cfg.Provider.Retry.Count).
		SetRetryWaitTime(s.cfg.Provider.Retry.Wait()).
		SetRetryMaxWaitTime(s.cfg.Provider.Retry.MaxWait()).
		AddRetryCondition(func(r *resty.Response, err error) bool {
			if err != nil {
				return true
			}
			if r == nil {
				return true
			}
			return r.StatusCode() >= 500
		})

	proxy := strings.TrimSpace(s.cfg.Proxy.Global)
	if proxy != "" {
		client.SetProxy(proxy)
	}

	ua := strings.TrimSpace(userAgent)
	if ua == "" {
		ua = strings.TrimSpace(s.cfg.Provider.UserAgent)
	}
	client.SetHeader("User-Agent", utils.NormalizeWXAppUserAgent(ua))
	client.SetHeader("device-type", "WXAPP")
	client.SetHeader("tenantId", defaultTenantID)
	client.SetHeader("x-requested-with", "XMLHttpRequest")

	client.SetHeader("Authorization", "Bearer "+strings.TrimSpace(token))
	client.SetHeader("token", strings.TrimSpace(token))
	client.SetHeader("x-token", strings.TrimSpace(token))

	resp, err := client.R().SetContext(ctx).Get(u.String())
	if err != nil {
		return "", err
	}
	if resp.StatusCode() >= 400 {
		return "", fmt.Errorf("current-user status %d", resp.StatusCode())
	}
	return extractCurrentUserUsername(resp.Body()), nil
}

func extractCurrentUserUsername(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return ""
	}
	if s, ok := m["success"].(bool); ok && !s {
		return ""
	}
	data, _ := m["data"].(map[string]any)
	if data == nil {
		return ""
	}
	switch v := data["username"].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprint(int64(v))
		}
		return fmt.Sprint(v)
	default:
		return ""
	}
}

func extractLoginToken(body []byte) (string, error) {
	if len(body) == 0 {
		return "", nil
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return "", err
	}
	if s, ok := m["success"].(bool); ok && !s {
		return "", nil
	}

	candidates := []any{
		m["token"],
	}

	if data, _ := m["data"].(map[string]any); data != nil {
		candidates = append(candidates,
			data["token"],
			data["accessToken"],
			data["access_token"],
			data["jwt"],
		)
		if extra, _ := data["extra"].(map[string]any); extra != nil {
			candidates = append(candidates,
				extra["token"],
				extra["accessToken"],
				extra["access_token"],
				extra["jwt"],
			)
		}
	}

	for _, c := range candidates {
		if v, ok := c.(string); ok && strings.TrimSpace(v) != "" {
			return v, nil
		}
	}
	return "", nil
}

func extractToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	if v := strings.TrimSpace(r.Header.Get("token")); v != "" {
		return v
	}
	if v := strings.TrimSpace(r.Header.Get("x-token")); v != "" {
		return v
	}
	return ""
}

func buildUpstreamURL(base, path, rawQuery string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return nil, err
	}
	basePath := strings.TrimRight(u.Path, "/")
	u.Path = basePath + path
	u.RawQuery = rawQuery
	return u, nil
}

func (s *Server) newUpstreamClient(account model.Account) (*resty.Client, *cookiejar.Jar, *url.URL, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, nil, err
	}
	importCookies(jar, account.Cookies)

	baseURL, err := url.Parse(strings.TrimSpace(s.cfg.Provider.BaseURL))
	if err != nil {
		return nil, nil, nil, err
	}

	client := resty.New().
		SetTimeout(s.cfg.Provider.Timeout()).
		SetCookieJar(jar).
		SetRetryCount(s.cfg.Provider.Retry.Count).
		SetRetryWaitTime(s.cfg.Provider.Retry.Wait()).
		SetRetryMaxWaitTime(s.cfg.Provider.Retry.MaxWait()).
		AddRetryCondition(func(r *resty.Response, err error) bool {
			if err != nil {
				return true
			}
			if r == nil {
				return true
			}
			return r.StatusCode() >= 500
		})

	proxy := strings.TrimSpace(account.Proxy)
	if proxy == "" {
		proxy = strings.TrimSpace(s.cfg.Proxy.Global)
	}
	if proxy != "" {
		client.SetProxy(proxy)
	}

	ua := strings.TrimSpace(account.UserAgent)
	if ua == "" {
		ua = strings.TrimSpace(s.cfg.Provider.UserAgent)
	}
	client.SetHeader("User-Agent", utils.NormalizeWXAppUserAgent(ua))
	client.SetHeader("device-type", "WXAPP")
	client.SetHeader("tenantId", defaultTenantID)
	client.SetHeader("x-requested-with", "XMLHttpRequest")
	if account.Token != "" {
		client.SetHeader("Authorization", "Bearer "+account.Token)
		client.SetHeader("token", account.Token)
		client.SetHeader("x-token", account.Token)
	}

	client.OnBeforeRequest(func(_ *resty.Client, req *resty.Request) error {
		if s.bus != nil {
			s.bus.Log("debug", "代理请求", map[string]any{
				"method": req.Method,
				"url":    req.URL,
			})
		}
		return nil
	})

	return client, jar, baseURL, nil
}

func importCookies(jar *cookiejar.Jar, entries []model.CookieJarEntry) {
	for _, entry := range entries {
		u, err := url.Parse(entry.URL)
		if err != nil {
			continue
		}
		jar.SetCookies(u, model.CookiesToHTTP(entry.Cookies))
	}
}

func exportCookies(baseURL *url.URL, jar *cookiejar.Jar) []model.CookieJarEntry {
	if baseURL == nil {
		return nil
	}
	u := *baseURL
	u.Path = "/"
	u.RawQuery = ""
	cookies := jar.Cookies(&u)
	return []model.CookieJarEntry{
		{URL: u.String(), Cookies: model.CookiesFromHTTP(cookies)},
	}
}
