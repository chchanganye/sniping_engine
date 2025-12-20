package notify

import (
	"bytes"
	"context"
	"errors"
	"html/template"
	"net/mail"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/gomail.v2"

	"sniping_engine/internal/logbus"
	"sniping_engine/internal/model"
	"sniping_engine/internal/store/sqlite"
)

type EmailNotifier struct {
	store *sqlite.Store
	bus   *logbus.Bus

	mu     sync.Mutex
	queue  chan OrderCreatedEvent
	ctx    context.Context
	cancel func()
	wg     sync.WaitGroup
}

func NewEmailNotifier(store *sqlite.Store, bus *logbus.Bus) *EmailNotifier {
	ctx, cancel := context.WithCancel(context.Background())
	n := &EmailNotifier{
		store:  store,
		bus:    bus,
		queue:  make(chan OrderCreatedEvent, 200),
		ctx:    ctx,
		cancel: cancel,
	}
	n.wg.Add(1)
	go n.loop()
	return n
}

func (n *EmailNotifier) Close(ctx context.Context) error {
	n.mu.Lock()
	cancel := n.cancel
	n.cancel = nil
	n.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	done := make(chan struct{})
	go func() {
		n.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (n *EmailNotifier) NotifyOrderCreated(_ context.Context, evt OrderCreatedEvent) {
	select {
	case n.queue <- evt:
	default:
		if n.bus != nil {
			n.bus.Log("warn", "email notify dropped (queue full)", map[string]any{
				"targetId":  evt.TargetID,
				"accountId": evt.AccountID,
				"orderId":   evt.OrderID,
			})
		}
	}
}

func (n *EmailNotifier) loop() {
	defer n.wg.Done()

	for {
		select {
		case <-n.ctx.Done():
			return
		case evt := <-n.queue:
			n.handle(evt)
		}
	}
}

func (n *EmailNotifier) handle(evt OrderCreatedEvent) {
	if n.store == nil {
		return
	}

	settings, ok, err := n.store.GetEmailSettings(n.ctx)
	if err != nil {
		if n.bus != nil {
			n.bus.Log("warn", "load email settings failed", map[string]any{"error": err.Error()})
		}
		return
	}
	if !ok || !settings.Enabled {
		return
	}

	if err := validateEmailSettings(settings); err != nil {
		if n.bus != nil {
			n.bus.Log("warn", "email settings invalid", map[string]any{"error": err.Error()})
		}
		return
	}

	if err := SendOrderCreatedEmail(n.ctx, settings, evt); err != nil {
		if n.bus != nil {
			n.bus.Log("warn", "email send failed", map[string]any{
				"error":     err.Error(),
				"targetId":  evt.TargetID,
				"accountId": evt.AccountID,
				"orderId":   evt.OrderID,
			})
		}
		return
	}

	if n.bus != nil {
		n.bus.Log("info", "email sent", map[string]any{
			"targetId":  evt.TargetID,
			"accountId": evt.AccountID,
			"orderId":   evt.OrderID,
			"to":        strings.TrimSpace(settings.Email),
		})
	}
}

func validateEmailSettings(s model.EmailSettings) error {
	email := strings.TrimSpace(s.Email)
	if email == "" {
		return errors.New("email is required")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return errors.New("invalid email")
	}
	if strings.TrimSpace(s.AuthCode) == "" {
		return errors.New("authCode is required")
	}
	return nil
}

func SendOrderCreatedEmail(ctx context.Context, settings model.EmailSettings, evt OrderCreatedEvent) error {
	if err := validateEmailSettings(settings); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	email := strings.TrimSpace(settings.Email)
	host, port, useSSL, err := smtpConfigForEmail(email)
	if err != nil {
		return err
	}
	subject := buildSubject(evt)
	htmlBody, textBody, err := buildEmailBody(evt)
	if err != nil {
		return err
	}

	msg := gomail.NewMessage()
	msg.SetHeader("From", msg.FormatAddress(email, "sniping_engine"))
	msg.SetHeader("To", email)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/plain", textBody)
	msg.AddAlternative("text/html", htmlBody)

	d := gomail.NewDialer(host, port, email, strings.TrimSpace(settings.AuthCode))
	d.SSL = useSSL
	return d.DialAndSend(msg)
}

func smtpConfigForEmail(email string) (host string, port int, useSSL bool, err error) {
	parts := strings.Split(strings.TrimSpace(email), "@")
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return "", 0, false, errors.New("invalid email format")
	}
	domain := strings.ToLower(strings.TrimSpace(parts[1]))

	switch {
	case domain == "qq.com" || strings.HasSuffix(domain, ".qq.com") || domain == "foxmail.com" || strings.HasSuffix(domain, ".foxmail.com"):
		return "smtp.qq.com", 465, true, nil
	case domain == "163.com" || strings.HasSuffix(domain, ".163.com") ||
		domain == "126.com" || strings.HasSuffix(domain, ".126.com") ||
		domain == "yeah.net" || strings.HasSuffix(domain, ".yeah.net"):
		return "smtp.163.com", 465, true, nil
	case domain == "gmail.com" || strings.HasSuffix(domain, ".gmail.com"):
		return "smtp.gmail.com", 587, false, nil
	case domain == "outlook.com" || strings.HasSuffix(domain, ".outlook.com") ||
		domain == "hotmail.com" || strings.HasSuffix(domain, ".hotmail.com") ||
		domain == "live.com" || strings.HasSuffix(domain, ".live.com"):
		return "smtp.office365.com", 587, false, nil
	case domain == "sina.com" || strings.HasSuffix(domain, ".sina.com"):
		return "smtp.sina.com", 465, true, nil
	case domain == "sohu.com" || strings.HasSuffix(domain, ".sohu.com"):
		return "smtp.sohu.com", 465, true, nil
	case domain == "aliyun.com" || strings.HasSuffix(domain, ".aliyun.com"):
		return "smtp.aliyun.com", 465, true, nil
	default:
		return "smtp." + domain, 465, true, nil
	}
}

func buildSubject(evt OrderCreatedEvent) string {
	name := strings.TrimSpace(evt.TargetName)
	if name == "" {
		name = "未知商品"
	}
	qty := evt.Quantity
	if qty <= 0 {
		qty = 1
	}
	return "抢购成功｜" + name + " × " + strconv.Itoa(qty)
}

var emailHTMLTpl = template.Must(template.New("email").Parse(`
<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width" />
    <title>抢购成功</title>
  </head>
  <body style="margin:0;padding:0;background:#f6f8fb;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,'PingFang SC','Hiragino Sans GB','Microsoft YaHei',sans-serif;">
    <div style="max-width:720px;margin:0 auto;padding:24px;">
      <div style="background:#ffffff;border:1px solid #e6e8ef;border-radius:14px;overflow:hidden;">
        <div style="padding:18px 22px;background:linear-gradient(135deg,#0ea5e9,#6366f1);color:#ffffff;">
          <div style="font-size:16px;font-weight:700;letter-spacing:.2px;">抢购成功</div>
          <div style="margin-top:6px;font-size:12px;opacity:.95;">sniping_engine 通知</div>
        </div>

        <div style="padding:22px;">
          <div style="font-size:18px;font-weight:700;color:#111827;line-height:1.35;">{{ .TargetName }}</div>
          <div style="margin-top:6px;color:#6b7280;font-size:12px;line-height:1.6;">
            订单号：<span style="color:#111827;font-weight:600;">{{ .OrderID }}</span>
            {{ if .TraceID }}<span style="margin-left:10px;">Trace：{{ .TraceID }}</span>{{ end }}
          </div>

          <div style="margin-top:16px;border:1px solid #eef0f6;border-radius:12px;overflow:hidden;">
            <table role="presentation" cellspacing="0" cellpadding="0" border="0" style="width:100%;border-collapse:collapse;">
              <tbody>
                {{ range .Rows }}
                <tr>
                  <td style="width:160px;padding:12px 14px;background:#fafbff;border-bottom:1px solid #eef0f6;color:#6b7280;font-size:12px;">{{ .K }}</td>
                  <td style="padding:12px 14px;border-bottom:1px solid #eef0f6;color:#111827;font-size:12px;font-weight:600;">{{ .V }}</td>
                </tr>
                {{ end }}
              </tbody>
            </table>
          </div>

          <div style="margin-top:14px;color:#9ca3af;font-size:12px;line-height:1.6;">
            如果你未发起此操作，请检查账号与 Token 配置。
          </div>
        </div>
      </div>
      <div style="text-align:center;margin-top:12px;color:#9ca3af;font-size:12px;">
        © sniping_engine
      </div>
    </div>
  </body>
</html>
`))

type rowKV struct {
	K string
	V string
}

func buildEmailBody(evt OrderCreatedEvent) (htmlBody string, textBody string, err error) {
	name := strings.TrimSpace(evt.TargetName)
	if name == "" {
		name = "未知商品"
	}
	mode := strings.TrimSpace(evt.Mode)
	if mode == "" {
		mode = "-"
	}
	qty := evt.Quantity
	if qty <= 0 {
		qty = 1
	}

	at := time.Now()
	if evt.At > 0 {
		at = time.UnixMilli(evt.At)
	}

	rows := []rowKV{
		{K: "时间", V: at.Format("2006-01-02 15:04:05")},
		{K: "账号", V: safeText(evt.Mobile, evt.AccountID)},
		{K: "模式", V: mode},
		{K: "数量", V: strconv.Itoa(qty)},
		{K: "itemId / skuId", V: joinIDs(evt.ItemID, evt.SKUID)},
		{K: "shopId", V: strconv.FormatInt(evt.ShopID, 10)},
		{K: "任务ID", V: evt.TargetID},
	}

	data := struct {
		TargetName string
		OrderID    string
		TraceID    string
		Rows       []rowKV
	}{
		TargetName: name,
		OrderID:    evt.OrderID,
		TraceID:    evt.TraceID,
		Rows:       rows,
	}

	var buf bytes.Buffer
	if err := emailHTMLTpl.Execute(&buf, data); err != nil {
		return "", "", err
	}

	text := new(strings.Builder)
	text.WriteString("抢购成功\n")
	text.WriteString("商品：" + name + "\n")
	if evt.OrderID != "" {
		text.WriteString("订单号：" + evt.OrderID + "\n")
	}
	if evt.TraceID != "" {
		text.WriteString("Trace：" + evt.TraceID + "\n")
	}
	for _, r := range rows {
		text.WriteString(r.K + "：" + r.V + "\n")
	}

	return buf.String(), text.String(), nil
}

func safeText(prefer, fallback string) string {
	prefer = strings.TrimSpace(prefer)
	if prefer != "" {
		return prefer
	}
	return strings.TrimSpace(fallback)
}

func joinIDs(itemID, skuID int64) string {
	item := strconv.FormatInt(itemID, 10)
	sku := strconv.FormatInt(skuID, 10)
	if item == "0" && sku == "0" {
		return "-"
	}
	return item + " / " + sku
}
