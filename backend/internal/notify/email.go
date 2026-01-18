package notify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/mail"
	"os"
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

	summaryWindow time.Duration
	maxBatch      int
}

func NewEmailNotifier(store *sqlite.Store, bus *logbus.Bus) *EmailNotifier {
	ctx, cancel := context.WithCancel(context.Background())
	n := &EmailNotifier{
		store:  store,
		bus:    bus,
		queue:  make(chan OrderCreatedEvent, 200),
		ctx:    ctx,
		cancel: cancel,
		summaryWindow: emailSummaryWindow(),
		maxBatch:      80,
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
			n.bus.Log("warn", "邮件通知丢弃：队列已满", map[string]any{
				"targetId":  evt.TargetID,
				"accountId": evt.AccountID,
				"orderId":   evt.OrderID,
			})
		}
	}
}

func (n *EmailNotifier) loop() {
	defer n.wg.Done()

	var (
		pending []OrderCreatedEvent
		timer   *time.Timer
		timerCh <-chan time.Time
	)

	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer = nil
		timerCh = nil
	}

	resetTimer := func() {
		if n.summaryWindow <= 0 {
			return
		}
		if timer == nil {
			timer = time.NewTimer(n.summaryWindow)
			timerCh = timer.C
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(n.summaryWindow)
	}

	flush := func(reason string) {
		if len(pending) == 0 {
			stopTimer()
			return
		}
		events := append([]OrderCreatedEvent(nil), pending...)
		pending = pending[:0]
		stopTimer()
		n.handleBatch(reason, events)
	}

	for {
		select {
		case <-n.ctx.Done():
			flush("shutdown")
			return
		case evt := <-n.queue:
			pending = append(pending, evt)
			if n.maxBatch > 0 && len(pending) >= n.maxBatch {
				flush("max")
				continue
			}
			if n.summaryWindow <= 0 {
				flush("immediate")
				continue
			}
			resetTimer()
		case <-timerCh:
			flush("idle")
		}
	}
}

func (n *EmailNotifier) handleBatch(reason string, events []OrderCreatedEvent) {
	if n.store == nil {
		return
	}

	settings, ok, err := n.store.GetEmailSettings(n.ctx)
	if err != nil {
		if n.bus != nil {
			n.bus.Log("warn", "读取邮件配置失败", map[string]any{"error": err.Error()})
		}
		return
	}
	if !ok || !settings.Enabled {
		if n.bus != nil {
			n.bus.Log("info", "邮件通知未启用", map[string]any{
				"count":  len(events),
				"reason": reason,
			})
		}
		return
	}

	if err := validateEmailSettings(settings); err != nil {
		if n.bus != nil {
			n.bus.Log("warn", "邮件配置无效", map[string]any{"error": err.Error()})
		}
		return
	}

	if err := SendOrderSummaryEmail(n.ctx, settings, events); err != nil {
		if n.bus != nil {
			n.bus.Log("warn", "邮件发送失败", map[string]any{
				"error":  err.Error(),
				"count":  len(events),
				"reason": reason,
			})
		}
		return
	}

	if n.bus != nil {
		n.bus.Log("info", "通知邮件已发送", map[string]any{
			"count":  len(events),
			"reason": reason,
			"to":     strings.TrimSpace(settings.Email),
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
	msg.SetHeader("From", msg.FormatAddress(email, "抢购助手"))
	msg.SetHeader("To", email)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/plain", textBody)
	msg.AddAlternative("text/html", htmlBody)

	d := gomail.NewDialer(host, port, email, strings.TrimSpace(settings.AuthCode))
	d.SSL = useSSL
	return d.DialAndSend(msg)
}

func SendOrderSummaryEmail(ctx context.Context, settings model.EmailSettings, events []OrderCreatedEvent) error {
	if err := validateEmailSettings(settings); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(events) == 0 {
		return errors.New("no events")
	}

	email := strings.TrimSpace(settings.Email)
	host, port, useSSL, err := smtpConfigForEmail(email)
	if err != nil {
		return err
	}
	subject := buildSummarySubject(events)
	htmlBody, textBody, err := buildSummaryEmailBody(events)
	if err != nil {
		return err
	}

	msg := gomail.NewMessage()
	msg.SetHeader("From", msg.FormatAddress(email, "抢购助手"))
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
	return fmt.Sprintf("下单成功（%s）：%s × %d", modeLabel(evt.Mode), name, qty)
}

func buildSummarySubject(events []OrderCreatedEvent) string {
	if len(events) == 0 {
		return "抢购结果汇总"
	}
	return fmt.Sprintf("抢购结果汇总（%d单）", len(events))
}

var emailHTMLTpl = template.Must(template.New("email").Parse(`
<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width" />
    <title>下单成功</title>
  </head>
  <body style="margin:0;padding:0;background:#f6f8fb;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,'PingFang SC','Hiragino Sans GB','Microsoft YaHei',sans-serif;">
    <div style="max-width:720px;margin:0 auto;padding:24px;">
      <div style="background:#ffffff;border:1px solid #e6e8ef;border-radius:14px;overflow:hidden;">
        <div style="padding:18px 22px;background:linear-gradient(135deg,#0ea5e9,#6366f1);color:#ffffff;">
          <div style="font-size:16px;font-weight:700;letter-spacing:.2px;">下单成功</div>
          <div style="margin-top:6px;font-size:12px;opacity:.95;">抢购助手通知</div>
        </div>

        <div style="padding:22px;">
          <div style="font-size:18px;font-weight:700;color:#111827;line-height:1.35;">{{ .TargetName }}</div>
          <div style="margin-top:6px;color:#6b7280;font-size:12px;line-height:1.6;">
            订单号：<span style="color:#111827;font-weight:600;">{{ .OrderID }}</span>
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
            此邮件由系统自动发送
          </div>
        </div>
      </div>
      <div style="text-align:center;margin-top:12px;color:#9ca3af;font-size:12px;">
        © 抢购助手
      </div>
    </div>
  </body>
</html>
`))

var emailSummaryHTMLTpl = template.Must(template.New("email-summary").Parse(`
<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width" />
    <title>抢购结果汇总</title>
  </head>
  <body style="margin:0;padding:0;background:#f6f8fb;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,'PingFang SC','Hiragino Sans GB','Microsoft YaHei',sans-serif;">
    <div style="max-width:720px;margin:0 auto;padding:24px;">
      <div style="background:#ffffff;border:1px solid #e6e8ef;border-radius:14px;overflow:hidden;">
        <div style="padding:18px 22px;background:linear-gradient(135deg,#0ea5e9,#6366f1);color:#ffffff;">
          <div style="font-size:16px;font-weight:700;letter-spacing:.2px;">抢购结果汇总</div>
          <div style="margin-top:6px;font-size:12px;opacity:.95;">抢购助手通知</div>
        </div>

        <div style="padding:22px;">
          <div style="font-size:14px;color:#111827;">
            共 <strong>{{ .Total }}</strong> 单，时间范围：{{ .Start }} ~ {{ .End }}
          </div>

          <div style="margin-top:12px;border:1px solid #eef0f6;border-radius:12px;overflow:hidden;">
            <table role="presentation" cellspacing="0" cellpadding="0" border="0" style="width:100%;border-collapse:collapse;">
              <thead>
                <tr style="background:#fafbff;">
                  <th style="padding:10px 12px;text-align:left;font-size:12px;color:#6b7280;border-bottom:1px solid #eef0f6;">时间</th>
                  <th style="padding:10px 12px;text-align:left;font-size:12px;color:#6b7280;border-bottom:1px solid #eef0f6;">商品</th>
                  <th style="padding:10px 12px;text-align:left;font-size:12px;color:#6b7280;border-bottom:1px solid #eef0f6;">账号</th>
                  <th style="padding:10px 12px;text-align:left;font-size:12px;color:#6b7280;border-bottom:1px solid #eef0f6;">数量</th>
                  <th style="padding:10px 12px;text-align:left;font-size:12px;color:#6b7280;border-bottom:1px solid #eef0f6;">订单号</th>
                </tr>
              </thead>
              <tbody>
                {{ range .Rows }}
                <tr>
                  <td style="padding:10px 12px;font-size:12px;color:#111827;border-bottom:1px solid #eef0f6;">{{ .At }}</td>
                  <td style="padding:10px 12px;font-size:12px;color:#111827;border-bottom:1px solid #eef0f6;">{{ .Target }}</td>
                  <td style="padding:10px 12px;font-size:12px;color:#111827;border-bottom:1px solid #eef0f6;">{{ .Account }}</td>
                  <td style="padding:10px 12px;font-size:12px;color:#111827;border-bottom:1px solid #eef0f6;">{{ .Qty }}</td>
                  <td style="padding:10px 12px;font-size:12px;color:#111827;border-bottom:1px solid #eef0f6;">{{ .OrderID }}</td>
                </tr>
                {{ end }}
              </tbody>
            </table>
          </div>

          <div style="margin-top:14px;color:#9ca3af;font-size:12px;line-height:1.6;">
            此邮件由系统自动发送
          </div>
        </div>
      </div>
      <div style="text-align:center;margin-top:12px;color:#9ca3af;font-size:12px;">
        © 抢购助手
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
		{K: "模式", V: modeLabel(evt.Mode)},
		{K: "数量", V: strconv.Itoa(qty)},
	}

	data := struct {
		TargetName string
		OrderID    string
		Rows       []rowKV
	}{
		TargetName: name,
		OrderID:    evt.OrderID,
		Rows:       rows,
	}

	var buf bytes.Buffer
	if err := emailHTMLTpl.Execute(&buf, data); err != nil {
		return "", "", err
	}

	text := new(strings.Builder)
	text.WriteString("下单成功\n")
	text.WriteString("商品：" + name + "\n")
	if evt.OrderID != "" {
		text.WriteString("订单号：" + evt.OrderID + "\n")
	}
	for _, r := range rows {
		text.WriteString(r.K + "：" + r.V + "\n")
	}

	return buf.String(), text.String(), nil
}

func buildSummaryEmailBody(events []OrderCreatedEvent) (htmlBody string, textBody string, err error) {
	if len(events) == 0 {
		return "", "", errors.New("no events")
	}

	type summaryRow struct {
		At      string
		Target  string
		Account string
		Qty     string
		OrderID string
	}

	rows := make([]summaryRow, 0, len(events))
	var (
		minAt time.Time
		maxAt time.Time
	)
	for i, evt := range events {
		at := time.Now()
		if evt.At > 0 {
			at = time.UnixMilli(evt.At)
		}
		if i == 0 || at.Before(minAt) {
			minAt = at
		}
		if i == 0 || at.After(maxAt) {
			maxAt = at
		}

		name := strings.TrimSpace(evt.TargetName)
		if name == "" {
			name = "未知商品"
		}
		qty := evt.Quantity
		if qty <= 0 {
			qty = 1
		}

		rows = append(rows, summaryRow{
			At:      at.Format("2006-01-02 15:04:05"),
			Target:  name,
			Account: safeText(evt.Mobile, evt.AccountID),
			Qty:     strconv.Itoa(qty),
			OrderID: strings.TrimSpace(evt.OrderID),
		})
	}

	data := struct {
		Total int
		Start string
		End   string
		Rows  []summaryRow
	}{
		Total: len(events),
		Start: minAt.Format("2006-01-02 15:04:05"),
		End:   maxAt.Format("2006-01-02 15:04:05"),
		Rows:  rows,
	}

	var buf bytes.Buffer
	if err := emailSummaryHTMLTpl.Execute(&buf, data); err != nil {
		return "", "", err
	}

	text := new(strings.Builder)
	text.WriteString("抢购结果汇总\n")
	text.WriteString(fmt.Sprintf("共 %d 单，时间范围：%s ~ %s\n", len(events), data.Start, data.End))
	for _, row := range rows {
		text.WriteString(fmt.Sprintf("- %s | %s | %s | 数量 %s | 订单 %s\n", row.At, row.Target, row.Account, row.Qty, row.OrderID))
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

func modeLabel(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "scan":
		return "扫货"
	case "rush":
		return "抢购"
	default:
		return "抢购"
	}
}

func emailSummaryWindow() time.Duration {
	v := strings.TrimSpace(os.Getenv("SNIPING_ENGINE_EMAIL_SUMMARY_SECONDS"))
	if v == "" {
		return 20 * time.Second
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 20 * time.Second
	}
	if n <= 0 {
		return 0
	}
	if n > 600 {
		n = 600
	}
	return time.Duration(n) * time.Second
}
