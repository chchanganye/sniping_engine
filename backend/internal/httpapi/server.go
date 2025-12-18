package httpapi

import (
	"context"
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
	"sniping_engine/internal/ws"
)

type Options struct {
	Cfg      config.Config
	Bus      *logbus.Bus
	Store    *sqlite.Store
	Engine   *engine.Engine
	Notifier notify.Notifier
}

type Server struct {
	cfg    config.Config
	bus    *logbus.Bus
	store  *sqlite.Store
	engine *engine.Engine
	notif  notify.Notifier
	ws     *ws.Handler
}

func New(opts Options) *Server {
	return &Server{
		cfg:    opts.Cfg,
		bus:    opts.Bus,
		store:  opts.Store,
		engine: opts.Engine,
		notif:  opts.Notifier,
		ws:     ws.NewHandler(opts.Bus, opts.Cfg.Server.Cors.AllowOrigins),
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
	api.HandleFunc("/api/v1/settings/email", s.handleEmailSettings)
	api.HandleFunc("/api/v1/settings/email/test", s.handleEmailTest)
	api.HandleFunc("/api/", s.handleUpstreamProxy)

	mux.Handle("/api/", corsMiddleware(s.cfg.Server.Cors, api))
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
		var body model.Account
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		acc, err := s.store.UpsertAccount(r.Context(), body)
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
		var body model.Target
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		t, err := s.store.UpsertTarget(r.Context(), body)
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
		masked := val
		if masked.AuthCode != "" {
			masked.AuthCode = "******"
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": masked})
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
		resp := saved
		if resp.AuthCode != "" {
			resp.AuthCode = "******"
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": resp})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleEmailTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	val, ok, err := s.store.GetEmailSettings(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if !ok || !val.Enabled {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "email is disabled"})
		return
	}
	if strings.TrimSpace(val.Email) == "" || strings.TrimSpace(val.AuthCode) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "email/authCode is required"})
		return
	}
	if s.notif == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "notifier unavailable"})
		return
	}
	s.notif.NotifyOrderCreated(r.Context(), notify.OrderCreatedEvent{
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
	})
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

	token := extractToken(r)
	if token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing token (Authorization/token/x-token)"})
		return
	}

	acc, err := s.store.GetAccountByToken(r.Context(), token)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "account not found for token"})
		return
	}
	acc.Token = token

	client, jar, baseURL, err := s.newUpstreamClient(acc)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
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

	acc.Cookies = exportCookies(baseURL, jar)
	_, _ = s.store.UpsertAccount(r.Context(), acc)

	if ct := strings.TrimSpace(resp.Header().Get("Content-Type")); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(resp.StatusCode())
	_, _ = w.Write(resp.Body())
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
	if ua != "" {
		client.SetHeader("User-Agent", ua)
	}
	if account.Token != "" {
		client.SetHeader("Authorization", "Bearer "+account.Token)
		client.SetHeader("token", account.Token)
		client.SetHeader("x-token", account.Token)
	}

	client.OnBeforeRequest(func(_ *resty.Client, req *resty.Request) error {
		if s.bus != nil {
			s.bus.Log("debug", "proxy request", map[string]any{
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
