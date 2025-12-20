package standard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"

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

const (
	tradeDeviceSourceWXAPP  = "WXAPP"
	tradeOrderSourceProduct = "product.detail.page"
)

type tradeBuyConfig struct {
	LineGrouped    bool `json:"lineGrouped"`
	MultipleCoupon bool `json:"multipleCoupon"`
}

type tradeRenderOrderLine struct {
	SKUID          int64          `json:"skuId"`
	ItemID         int64          `json:"itemId"`
	Quantity       int            `json:"quantity"`
	PromotionTag   any            `json:"promotionTag"`
	ActivityID     any            `json:"activityId"`
	Extra          map[string]any `json:"extra"`
	ShopID         int64          `json:"shopId"`
	ShopActivityID any            `json:"shopActivityId,omitempty"`
}

type tradeRenderOrderRequest struct {
	DeviceSource  string                 `json:"deviceSource"`
	OrderSource   string                 `json:"orderSource"`
	BuyConfig     tradeBuyConfig         `json:"buyConfig"`
	ItemName      any                    `json:"itemName"`
	OrderLineList []tradeRenderOrderLine `json:"orderLineList"`
	DivisionIDs   string                 `json:"divisionIds,omitempty"`
	AddressID     *int64                 `json:"addressId"`
	CouponParams  []any                  `json:"couponParams"`
	BenefitParams []any                  `json:"benefitParams"`
	Delivery      map[string]any         `json:"delivery"`
	Extra         map[string]any         `json:"extra"`
	DevicesID     string                 `json:"devicesId,omitempty"`
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

	updated, err := p.ensureAccountTradeContext(ctx, client, account)
	if err != nil {
		return provider.PreflightResult{}, model.Account{}, err
	}

	devicesID := strings.TrimSpace(updated.DeviceID)
	if devicesID == "" {
		return provider.PreflightResult{}, model.Account{}, errors.New("deviceId is required")
	}

	qty := target.PerOrderQty
	if qty <= 0 {
		qty = 1
	}

	var addrPtr *int64
	if updated.AddressID > 0 {
		v := updated.AddressID
		addrPtr = &v
	}

	var itemName any = nil
	if strings.TrimSpace(target.Name) != "" {
		itemName = strings.TrimSpace(target.Name)
	}

	payload := tradeRenderOrderRequest{
		DeviceSource: tradeDeviceSourceWXAPP,
		OrderSource:  tradeOrderSourceProduct,
		BuyConfig:    tradeBuyConfig{LineGrouped: true, MultipleCoupon: true},
		ItemName:     itemName,
		OrderLineList: []tradeRenderOrderLine{
			{
				SKUID:        target.SKUID,
				ItemID:       target.ItemID,
				Quantity:     qty,
				PromotionTag: nil,
				ActivityID:   nil,
				Extra:        map[string]any{},
				ShopID:       target.ShopID,
			},
		},
		DivisionIDs:   strings.TrimSpace(updated.DivisionIDs),
		AddressID:     addrPtr,
		CouponParams:  []any{},
		BenefitParams: []any{},
		Delivery:      map[string]any{},
		Extra: map[string]any{
			"renewOriginOrderId":   "",
			"renewOriginAddressId": "",
			"activityGroupId":      nil,
		},
		DevicesID: devicesID,
	}

	var env apiEnvelope[json.RawMessage]
	resp, err := client.R().
		SetContext(ctx).
		SetBody(payload).
		SetResult(&env).
		Post("/api/trade/buy/render-order")
	if err != nil {
		return provider.PreflightResult{}, model.Account{}, err
	}
	if resp.StatusCode() >= 400 {
		msg := httpErrorSummary(resp)
		p.logUpstreamFailure("render-order", resp, msg, map[string]any{
			"accountId": account.ID,
			"targetId":  target.ID,
		})
		return provider.PreflightResult{}, model.Account{}, fmt.Errorf("render-order status %d: %s", resp.StatusCode(), msg)
	}
	if !env.Success {
		msg := strings.TrimSpace(env.Error)
		if msg == "" {
			msg = strings.TrimSpace(env.Message)
		}
		if msg == "" {
			msg = "render-order failed"
		}
		p.logUpstreamFailure("render-order", resp, msg, map[string]any{
			"accountId": account.ID,
			"targetId":  target.ID,
		})
		return provider.PreflightResult{}, model.Account{}, fmt.Errorf("render-order failed: %s", msg)
	}

	canBuy, totalFee := parseRenderCanBuyAndTotalFee(env.Data)
	needCaptcha := parseRenderNeedCaptcha(env.Data)

	updated.Cookies = p.exportCookies(jar)
	return provider.PreflightResult{
		CanBuy:      canBuy,
		NeedCaptcha: needCaptcha,
		TotalFee:    totalFee,
		Render:      env.Data,
	}, updated, nil
}

func (p *StandardProvider) CreateOrder(ctx context.Context, account model.Account, target model.Target, preflight provider.PreflightResult) (provider.CreateResult, model.Account, error) {
	client, jar, err := p.newClient(account)
	if err != nil {
		return provider.CreateResult{}, model.Account{}, err
	}
	if len(preflight.Render) == 0 {
		return provider.CreateResult{}, model.Account{}, errors.New("missing render data from preflight")
	}

	captchaVerifyParam := strings.TrimSpace(target.CaptchaVerifyParam)
	if preflight.NeedCaptcha {
		if captchaVerifyParam == "" {
			return provider.CreateResult{}, account, errors.New("需要验证码：请先为该目标任务配置 captchaVerifyParam")
		}
	} else {
		captchaVerifyParam = ""
	}

	payload, err := buildTradeCreateOrderPayloadFromRender(preflight.Render, strings.TrimSpace(target.Name), strings.TrimSpace(account.DeviceID), captchaVerifyParam)
	if err != nil {
		return provider.CreateResult{}, model.Account{}, err
	}

	var env apiEnvelope[json.RawMessage]
	resp, err := client.R().
		SetContext(ctx).
		SetBody(payload).
		SetResult(&env).
		Post("/api/trade/buy/create-order")
	if err != nil {
		return provider.CreateResult{}, model.Account{}, err
	}
	if resp.StatusCode() >= 400 {
		msg := httpErrorSummary(resp)
		p.logUpstreamFailure("create-order", resp, msg, map[string]any{
			"accountId": account.ID,
			"targetId":  target.ID,
		})
		return provider.CreateResult{}, model.Account{}, fmt.Errorf("create-order status %d: %s", resp.StatusCode(), msg)
	}
	if !env.Success {
		msg := strings.TrimSpace(env.Error)
		if msg == "" {
			msg = strings.TrimSpace(env.Message)
		}
		if msg == "" {
			msg = "create-order failed"
		}
		p.logUpstreamFailure("create-order", resp, msg, map[string]any{
			"accountId": account.ID,
			"targetId":  target.ID,
		})
		return provider.CreateResult{}, model.Account{}, fmt.Errorf("create-order failed: %s", msg)
	}

	orderID, traceID := extractCreateOrderIDs(env.Data)

	updated := account
	updated.Cookies = p.exportCookies(jar)

	return provider.CreateResult{
		Success: true,
		OrderID: orderID,
		TraceID: traceID,
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
			"app":        app,
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

func (p *StandardProvider) ensureAccountTradeContext(ctx context.Context, client *resty.Client, account model.Account) (model.Account, error) {
	if client == nil {
		return model.Account{}, errors.New("http client is required")
	}
	if account.AddressID > 0 && strings.TrimSpace(account.DivisionIDs) != "" {
		return account, nil
	}

	var env apiEnvelope[json.RawMessage]
	resp, err := client.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"app":        "o2o",
			"isAllCover": "1",
		}).
		SetResult(&env).
		Get("/api/user/web/shipping-address/self/list-all")
	if err != nil {
		return model.Account{}, err
	}
	if resp.StatusCode() >= 400 {
		return model.Account{}, fmt.Errorf("shipping-address status %d: %s", resp.StatusCode(), httpErrorSummary(resp))
	}
	if !env.Success {
		msg := strings.TrimSpace(env.Error)
		if msg == "" {
			msg = strings.TrimSpace(env.Message)
		}
		if msg == "" {
			msg = "fetch shipping address failed"
		}
		return model.Account{}, errors.New(msg)
	}

	var list []map[string]any
	if err := decodeUseNumber(env.Data, &list); err != nil {
		return model.Account{}, err
	}
	if len(list) == 0 {
		return model.Account{}, errors.New("no shipping address")
	}
	pick := list[0]
	for _, a := range list {
		if asBool(a["checked"]) {
			pick = a
			break
		}
	}
	if !asBool(pick["checked"]) {
		for _, a := range list {
			if asBool(a["isDefault"]) {
				pick = a
				break
			}
		}
	}

	id, ok := toInt64(pick["id"])
	if !ok || id <= 0 {
		return model.Account{}, errors.New("invalid address id")
	}

	next := account
	if next.AddressID <= 0 {
		next.AddressID = id
	}
	if strings.TrimSpace(next.DivisionIDs) == "" {
		next.DivisionIDs = resolveDivisionIDs(pick)
	}
	return next, nil
}

func parseRenderCanBuyAndTotalFee(renderData json.RawMessage) (canBuy bool, totalFee int64) {
	var m map[string]any
	if err := decodeUseNumber(renderData, &m); err != nil {
		return false, 0
	}

	if ps, ok := asMap(m["purchaseStatus"]); ok {
		if v, ok := ps["canBuy"].(bool); ok {
			canBuy = v
		}
	}

	if v, ok := toInt64(m["totalFee"]); ok {
		totalFee = v
		return canBuy, totalFee
	}
	if pi, ok := asMap(m["priceInfo"]); ok {
		if v, ok := toInt64(pi["totalFee"]); ok {
			totalFee = v
			return canBuy, totalFee
		}
	}
	return canBuy, 0
}

func parseRenderNeedCaptcha(renderData json.RawMessage) bool {
	var m map[string]any
	if err := decodeUseNumber(renderData, &m); err != nil {
		return false
	}

	if extra, ok := asMap(m["extra"]); ok {
		if isTruthy(extra["isCaptchaVerifyParam"]) {
			return true
		}
	}

	if lines, ok := asSlice(m["orderLineList"]); ok {
		for _, item := range lines {
			line, ok := asMap(item)
			if !ok {
				continue
			}
			if attrs, ok := asMap(line["itemAttributes"]); ok {
				if isTruthy(attrs["captchaVerify"]) {
					return true
				}
			}
		}
	}

	return false
}

func buildTradeCreateOrderPayloadFromRender(renderData json.RawMessage, fallbackItemName string, fallbackDevicesID string, captchaVerifyParam string) (map[string]any, error) {
	var render map[string]any
	if err := decodeUseNumber(renderData, &render); err != nil {
		return nil, err
	}

	deviceSource := tradeDeviceSourceWXAPP
	orderSource := tradeOrderSourceProduct
	if extra, ok := asMap(render["extra"]); ok {
		if v, ok := extra["orderSource"].(string); ok && strings.TrimSpace(v) != "" {
			orderSource = strings.TrimSpace(v)
		}
	}

	addressID := pickRenderAddressID(render)
	if addressID <= 0 {
		return nil, errors.New("render-order missing addressId")
	}

	if _, ok := render["orderList"]; !ok {
		return nil, errors.New("render-order missing orderList")
	}
	if _, ok := render["priceInfo"]; !ok {
		return nil, errors.New("render-order missing priceInfo")
	}

	totalFee, ok := pickRenderTotalFee(render)
	if !ok {
		return nil, errors.New("render-order missing totalFee")
	}

	extra := map[string]any{}
	if oldExtra, ok := asMap(render["extra"]); ok {
		for k, v := range oldExtra {
			extra[k] = v
		}
	}
	extra["deviceSource"] = deviceSource
	if strings.TrimSpace(captchaVerifyParam) != "" {
		extra["captchaVerifyParam"] = strings.TrimSpace(captchaVerifyParam)
	}

	itemName := pickRenderSkuName(render)
	if strings.TrimSpace(itemName) == "" && strings.TrimSpace(fallbackItemName) != "" {
		itemName = strings.TrimSpace(fallbackItemName)
	}

	render["deviceSource"] = deviceSource
	render["orderSource"] = orderSource
	render["buyConfig"] = tradeBuyConfig{LineGrouped: true, MultipleCoupon: true}
	render["itemName"] = itemName
	render["addressId"] = addressID
	render["totalFee"] = totalFee
	render["extra"] = extra

	if _, ok := render["devicesId"]; !ok {
		if v, ok := extra["devicesId"].(string); ok && strings.TrimSpace(v) != "" {
			render["devicesId"] = strings.TrimSpace(v)
		} else if strings.TrimSpace(fallbackDevicesID) != "" {
			render["devicesId"] = strings.TrimSpace(fallbackDevicesID)
		}
	}

	if render["shipFeeInfo"] == nil && render["shipFee"] != nil {
		render["shipFeeInfo"] = render["shipFee"]
	}

	return render, nil
}

func pickRenderAddressID(render map[string]any) int64 {
	list, ok := asSlice(render["addressInfoList"])
	if !ok || len(list) == 0 {
		return 0
	}
	var picked map[string]any
	for _, item := range list {
		m, ok := asMap(item)
		if !ok {
			continue
		}
		if asBool(m["checked"]) {
			picked = m
			break
		}
	}
	if picked == nil {
		for _, item := range list {
			m, ok := asMap(item)
			if !ok {
				continue
			}
			if asBool(m["isDefault"]) {
				picked = m
				break
			}
		}
	}
	if picked == nil {
		if m, ok := asMap(list[0]); ok {
			picked = m
		}
	}
	if picked == nil {
		return 0
	}
	id, ok := toInt64(picked["id"])
	if !ok {
		return 0
	}
	return id
}

func pickRenderSkuName(render map[string]any) string {
	if list, ok := asSlice(render["orderLineList"]); ok && len(list) > 0 {
		if line0, ok := asMap(list[0]); ok {
			if v, ok := line0["skuName"].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	if orderList, ok := asSlice(render["orderList"]); ok && len(orderList) > 0 {
		if order0, ok := asMap(orderList[0]); ok {
			name := deepGetString(order0, "activityOrderList", 0, "orderLineGroups", 0, "orderLineList", 0, "skuName")
			if strings.TrimSpace(name) != "" {
				return strings.TrimSpace(name)
			}
		}
	}
	return ""
}

func pickRenderTotalFee(render map[string]any) (int64, bool) {
	if v, ok := toInt64(render["totalFee"]); ok {
		return v, true
	}
	if pi, ok := asMap(render["priceInfo"]); ok {
		if v, ok := toInt64(pi["totalFee"]); ok {
			return v, true
		}
	}
	return 0, false
}

func extractCreateOrderIDs(createData json.RawMessage) (orderID string, traceID string) {
	var m map[string]any
	if err := decodeUseNumber(createData, &m); err != nil {
		return "", ""
	}
	if v, ok := m["traceId"].(string); ok {
		traceID = strings.TrimSpace(v)
	}

	if v, ok := toInt64(m["purchaseOrderId"]); ok && v > 0 {
		return strconv.FormatInt(v, 10), traceID
	}
	if v, ok := toInt64(m["orderId"]); ok && v > 0 {
		return strconv.FormatInt(v, 10), traceID
	}
	if infos, ok := asSlice(m["orderInfos"]); ok && len(infos) > 0 {
		if m0, ok := asMap(infos[0]); ok {
			if v, ok := toInt64(m0["orderId"]); ok && v > 0 {
				return strconv.FormatInt(v, 10), traceID
			}
		}
	}

	if v, ok := m["purchaseOrderId"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v), traceID
	}
	if v, ok := m["orderId"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v), traceID
	}

	return "", traceID
}

func resolveDivisionIDs(address map[string]any) string {
	candidates := []any{
		address["divisionIds"],
		address["divisionLevels"],
		address["divisionIdLevels"],
	}
	for _, c := range candidates {
		if v, ok := c.(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}

	if rawLevels, ok := address["divisionLevels"]; ok {
		if arr, ok := asSlice(rawLevels); ok && len(arr) > 0 {
			var parts []string
			for _, item := range arr {
				if n, ok := toInt64(item); ok {
					parts = append(parts, strconv.FormatInt(n, 10))
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, ",")
			}
		}
	}

	var parts []string
	for _, k := range []string{"provinceId", "cityId", "regionId"} {
		if n, ok := toInt64(address[k]); ok && n > 0 {
			parts = append(parts, strconv.FormatInt(n, 10))
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, ",")
	}
	return ""
}

func decodeUseNumber(b []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	return dec.Decode(out)
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func asSlice(v any) ([]any, bool) {
	s, ok := v.([]any)
	return s, ok
}

func asBool(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func toInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case json.Number:
		n, err := t.Int64()
		return n, err == nil
	case float64:
		if t == float64(int64(t)) {
			return int64(t), true
		}
		return 0, false
	case int64:
		return t, true
	case int:
		return int64(t), true
	case uint64:
		if t > uint64(^uint64(0)>>1) {
			return 0, false
		}
		return int64(t), true
	case string:
		if strings.TrimSpace(t) == "" {
			return 0, false
		}
		n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func isTruthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return false
		}
		return strings.EqualFold(s, "true") || s == "1"
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			return false
		}
		return n != 0
	case float64:
		return t != 0
	case int:
		return t != 0
	case int64:
		return t != 0
	default:
		return false
	}
}

func deepGetString(m map[string]any, path ...any) string {
	var cur any = m
	for _, p := range path {
		switch key := p.(type) {
		case string:
			nextMap, ok := asMap(cur)
			if !ok {
				return ""
			}
			cur = nextMap[key]
		case int:
			nextSlice, ok := asSlice(cur)
			if !ok || key < 0 || key >= len(nextSlice) {
				return ""
			}
			cur = nextSlice[key]
		default:
			return ""
		}
	}
	if s, ok := cur.(string); ok {
		return s
	}
	return ""
}

func httpErrorSummary(resp *resty.Response) string {
	if resp == nil {
		return ""
	}
	body := bytes.TrimSpace(resp.Body())
	if len(body) == 0 {
		return resp.Status()
	}

	var m map[string]any
	if err := decodeUseNumber(body, &m); err == nil {
		if v, ok := m["error"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		if v, ok := m["message"].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		if s, ok := m["success"].(bool); ok && !s {
			if v, ok := m["msg"].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}

	text := strings.TrimSpace(string(body))
	if text == "" {
		return resp.Status()
	}
	if len(text) > 400 {
		return text[:400] + "..."
	}
	return text
}

func (p *StandardProvider) logUpstreamFailure(api string, resp *resty.Response, msg string, fields map[string]any) {
	if p == nil || p.bus == nil || resp == nil {
		return
	}
	body := strings.TrimSpace(string(resp.Body()))
	if len(body) > 4000 {
		body = body[:4000] + "..."
	}
	out := map[string]any{
		"api":    api,
		"status": resp.StatusCode(),
		"error":  strings.TrimSpace(msg),
		"body":   body,
	}
	if resp.Request != nil {
		out["method"] = resp.Request.Method
		out["url"] = resp.Request.URL
	}
	for k, v := range fields {
		if v == nil {
			continue
		}
		out[k] = v
	}
	p.bus.Log("warn", "upstream request failed", out)
}
