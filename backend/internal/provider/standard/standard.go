package standard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/cookiejar"
	"net/url"
	"strconv"

	"github.com/go-resty/resty/v2"

	"sniping_engine/internal/config"
	"sniping_engine/internal/logbus"
	"sniping_engine/internal/model"
	"sniping_engine/internal/provider"
)

type StandardProvider struct {
	cfg      config.ProviderConfig
	proxyCfg config.ProxyConfig
	bus      *logbus.Bus
	baseURL  *url.URL
}

func New(cfg config.ProviderConfig, proxyCfg config.ProxyConfig, bus *logbus.Bus) *StandardProvider {
	u, _ := url.Parse(cfg.BaseURL)
	return &StandardProvider{
		cfg:      cfg,
		proxyCfg: proxyCfg,
		bus:      bus,
		baseURL:  u,
	}
}

func (p *StandardProvider) Name() string { return "standard" }

type apiEnvelope[T any] struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
	Code    any    `json:"code,omitempty"`
	Data    T      `json:"data"`
}

type loginBySMSReq struct {
	Mobile  string `json:"mobile"`
	SMSCode string `json:"smsCode"`
}

type loginBySMSResp struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Data    struct {
		Token    string `json:"token"`
		DeviceID string `json:"deviceId"`
		UUID     string `json:"uuid"`
	} `json:"data"`
}

type preflightReq struct {
	ItemID   int64 `json:"itemId"`
	SKUID    int64 `json:"skuId"`
	Quantity int   `json:"quantity"`
	ShopID   int64 `json:"shopId,omitempty"`
}

type preflightResp struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Data    struct {
		CanBuy   bool   `json:"canBuy"`
		TotalFee int64  `json:"totalFee"`
		TraceID  string `json:"traceId"`
	} `json:"data"`
}

type createOrderReq struct {
	ItemID   int64 `json:"itemId"`
	SKUID    int64 `json:"skuId"`
	Quantity int   `json:"quantity"`
	ShopID   int64 `json:"shopId,omitempty"`
	TotalFee int64 `json:"totalFee"`
	TraceID  string `json:"traceId,omitempty"`
}

type createOrderResp struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Data    struct {
		OrderID any    `json:"orderId"`
		TraceID string `json:"traceId,omitempty"`
	} `json:"data"`
}

func (p *StandardProvider) LoginBySMS(ctx context.Context, account model.Account, mobile, smsCode string) (model.Account, error) {
	client, jar, err := p.newClient(account)
	if err != nil {
		return model.Account{}, err
	}

	var resp loginBySMSResp
	_, err = client.R().
		SetContext(ctx).
		SetBody(loginBySMSReq{Mobile: mobile, SMSCode: smsCode}).
		SetResult(&resp).
		Post("/login-by-sms")
	if err != nil {
		return model.Account{}, err
	}
	if !resp.Success {
		if resp.Error == "" {
			resp.Error = "login failed"
		}
		return model.Account{}, errors.New(resp.Error)
	}

	updated := account
	updated.Mobile = mobile
	updated.Token = resp.Data.Token
	if updated.DeviceID == "" {
		updated.DeviceID = resp.Data.DeviceID
	}
	if updated.UUID == "" {
		updated.UUID = resp.Data.UUID
	}
	updated.Cookies = p.exportCookies(jar)
	return updated, nil
}

func (p *StandardProvider) Preflight(ctx context.Context, account model.Account, target model.Target) (provider.PreflightResult, model.Account, error) {
	client, jar, err := p.newClient(account)
	if err != nil {
		return provider.PreflightResult{}, model.Account{}, err
	}
	var resp preflightResp
	_, err = client.R().
		SetContext(ctx).
		SetBody(preflightReq{
			ItemID:   target.ItemID,
			SKUID:    target.SKUID,
			Quantity: target.PerOrderQty,
			ShopID:   target.ShopID,
		}).
		SetResult(&resp).
		Post("/preflight-order")
	if err != nil {
		return provider.PreflightResult{}, model.Account{}, err
	}
	if !resp.Success {
		if resp.Error == "" {
			resp.Error = "preflight failed"
		}
		return provider.PreflightResult{}, model.Account{}, errors.New(resp.Error)
	}

	updated := account
	updated.Cookies = p.exportCookies(jar)
	return provider.PreflightResult{
		CanBuy:   resp.Data.CanBuy,
		TotalFee: resp.Data.TotalFee,
		TraceID:  resp.Data.TraceID,
	}, updated, nil
}

func (p *StandardProvider) CreateOrder(ctx context.Context, account model.Account, target model.Target, preflight provider.PreflightResult) (provider.CreateResult, model.Account, error) {
	client, jar, err := p.newClient(account)
	if err != nil {
		return provider.CreateResult{}, model.Account{}, err
	}
	var resp createOrderResp
	_, err = client.R().
		SetContext(ctx).
		SetBody(createOrderReq{
			ItemID:   target.ItemID,
			SKUID:    target.SKUID,
			Quantity: target.PerOrderQty,
			ShopID:   target.ShopID,
			TotalFee: preflight.TotalFee,
			TraceID:  preflight.TraceID,
		}).
		SetResult(&resp).
		Post("/create-order")
	if err != nil {
		return provider.CreateResult{}, model.Account{}, err
	}
	if !resp.Success {
		if resp.Error == "" {
			resp.Error = "create order failed"
		}
		return provider.CreateResult{}, model.Account{}, errors.New(resp.Error)
	}

	updated := account
	updated.Cookies = p.exportCookies(jar)

	return provider.CreateResult{
		Success: true,
		OrderID: fmt.Sprint(resp.Data.OrderID),
		TraceID: resp.Data.TraceID,
	}, updated, nil
}

func (p *StandardProvider) GetShippingAddresses(ctx context.Context, account model.Account, params provider.ShippingAddressParams) (json.RawMessage, model.Account, error) {
	client, jar, err := p.newClient(account)
	if err != nil {
		return nil, model.Account{}, err
	}

	app := params.App
	if app == "" {
		app = "o2o"
	}

	var resp apiEnvelope[json.RawMessage]
	_, err = client.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"app":       app,
			"isAllCover": strconv.Itoa(params.IsAllCover),
		}).
		SetResult(&resp).
		Get("/api/user/web/shipping-address/self/list-all")
	if err != nil {
		return nil, model.Account{}, err
	}
	if !resp.Success {
		msg := resp.Error
		if msg == "" {
			msg = resp.Message
		}
		if msg == "" {
			msg = "get shipping addresses failed"
		}
		return nil, model.Account{}, errors.New(msg)
	}

	updated := account
	updated.Cookies = p.exportCookies(jar)
	return resp.Data, updated, nil
}

func (p *StandardProvider) GetCategoryTree(ctx context.Context, account model.Account, params provider.CategoryTreeParams) (json.RawMessage, model.Account, error) {
	client, jar, err := p.newClient(account)
	if err != nil {
		return nil, model.Account{}, err
	}

	var resp apiEnvelope[json.RawMessage]
	_, err = client.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"frontCategoryId": strconv.FormatInt(params.FrontCategoryID, 10),
			"longitude":       strconv.FormatFloat(params.Longitude, 'f', -1, 64),
			"latitude":        strconv.FormatFloat(params.Latitude, 'f', -1, 64),
			"isFinish":        strconv.FormatBool(params.IsFinish),
		}).
		SetResult(&resp).
		Get("/api/item/shop-category/tree")
	if err != nil {
		return nil, model.Account{}, err
	}
	if !resp.Success {
		msg := resp.Error
		if msg == "" {
			msg = resp.Message
		}
		if msg == "" {
			msg = "get category tree failed"
		}
		return nil, model.Account{}, errors.New(msg)
	}

	updated := account
	updated.Cookies = p.exportCookies(jar)
	return resp.Data, updated, nil
}

func (p *StandardProvider) GetStoreSkuByCategory(ctx context.Context, account model.Account, params provider.StoreSkuByCategoryParams) (json.RawMessage, model.Account, error) {
	client, jar, err := p.newClient(account)
	if err != nil {
		return nil, model.Account{}, err
	}

	pageNo := params.PageNo
	if pageNo <= 0 {
		pageNo = 1
	}
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	var resp apiEnvelope[json.RawMessage]
	_, err = client.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"pageNo":          strconv.Itoa(pageNo),
			"pageSize":        strconv.Itoa(pageSize),
			"frontCategoryId": strconv.FormatInt(params.FrontCategoryID, 10),
			"longitude":       strconv.FormatFloat(params.Longitude, 'f', -1, 64),
			"latitude":        strconv.FormatFloat(params.Latitude, 'f', -1, 64),
			"isFinish":        strconv.FormatBool(params.IsFinish),
		}).
		SetResult(&resp).
		Get("/api/item/store/item/searchStoreSkuByCategory")
	if err != nil {
		return nil, model.Account{}, err
	}
	if !resp.Success {
		msg := resp.Error
		if msg == "" {
			msg = resp.Message
		}
		if msg == "" {
			msg = "get store sku by category failed"
		}
		return nil, model.Account{}, errors.New(msg)
	}

	updated := account
	updated.Cookies = p.exportCookies(jar)
	return resp.Data, updated, nil
}

func (p *StandardProvider) newClient(account model.Account) (*resty.Client, *cookiejar.Jar, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, err
	}
	p.importCookies(jar, account.Cookies)

	client := resty.New().
		SetBaseURL(p.cfg.BaseURL).
		SetTimeout(p.cfg.Timeout()).
		SetCookieJar(jar).
		SetRetryCount(p.cfg.Retry.Count).
		SetRetryWaitTime(p.cfg.Retry.Wait()).
		SetRetryMaxWaitTime(p.cfg.Retry.MaxWait()).
		AddRetryCondition(func(r *resty.Response, err error) bool {
			if err != nil {
				return true
			}
			if r == nil {
				return true
			}
			return r.StatusCode() >= 500
		})

	proxy := account.Proxy
	if proxy == "" {
		proxy = p.proxyCfg.Global
	}
	if proxy != "" {
		client.SetProxy(proxy)
	}

	ua := account.UserAgent
	if ua == "" {
		ua = p.cfg.UserAgent
	}
	client.SetHeader("User-Agent", ua)
	if account.Token != "" {
		client.SetHeader("Authorization", "Bearer "+account.Token)
		client.SetHeader("token", account.Token)
		client.SetHeader("x-token", account.Token)
	}

	client.OnBeforeRequest(func(_ *resty.Client, req *resty.Request) error {
		if p.bus != nil {
			p.bus.Log("debug", "http request", map[string]any{
				"method": req.Method,
				"url":    req.URL,
			})
		}
		return nil
	})

	return client, jar, nil
}

func (p *StandardProvider) importCookies(jar *cookiejar.Jar, entries []model.CookieJarEntry) {
	for _, entry := range entries {
		u, err := url.Parse(entry.URL)
		if err != nil {
			continue
		}
		jar.SetCookies(u, model.CookiesToHTTP(entry.Cookies))
	}
}

func (p *StandardProvider) exportCookies(jar *cookiejar.Jar) []model.CookieJarEntry {
	if p.baseURL == nil {
		return nil
	}
	u := *p.baseURL
	u.Path = "/"
	cookies := jar.Cookies(&u)
	return []model.CookieJarEntry{
		{URL: u.String(), Cookies: model.CookiesFromHTTP(cookies)},
	}
}
