package httpapi

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const captchaManualSourceURL = "https://m.4008117117.com/aliyun-captcha&cookie=true"

var (
	captchaSceneIDRe = regexp.MustCompile(`SceneId:\s*"([^"]+)"`)
	captchaRegionRe  = regexp.MustCompile(`region:\s*"([^"]+)"`)
	captchaPrefixRe  = regexp.MustCompile(`prefix:\s*"([^"]+)"`)
)

type captchaManualConfig struct {
	SceneID string
	Region  string
	Prefix  string
}

var captchaManualPageTpl = template.Must(template.New("captcha-manual").Parse(`<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1, user-scalable=no" />
    <title>Manual Captcha</title>
    <style>
      body {
        margin: 0;
        background: #f5f5f5;
        font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif;
        display: flex;
        align-items: center;
        justify-content: center;
        height: 100vh;
        color: #303133;
      }
      .card {
        width: min(420px, 92vw);
        background: #fff;
        border-radius: 12px;
        padding: 18px 16px 20px;
        box-shadow: 0 6px 30px rgba(0, 0, 0, 0.08);
        text-align: center;
      }
      .title {
        font-size: 18px;
        font-weight: 600;
        margin-bottom: 12px;
      }
      #button {
        width: 100%;
        height: 44px;
        border: none;
        border-radius: 999px;
        background: #0054a7;
        color: #fff;
        font-size: 16px;
        cursor: pointer;
      }
      #status {
        margin-top: 12px;
        font-size: 12px;
        color: #909399;
        min-height: 18px;
      }
    </style>
    <script>
      window.AliyunCaptchaConfig = { region: "{{.Region}}", prefix: "{{.Prefix}}" };
    </script>
    <script src="https://o.alicdn.com/captcha-frontend/aliyunCaptcha/AliyunCaptcha.js"></script>
  </head>
  <body>
    <div class="card">
      <div class="title">Manual Captcha</div>
      <div id="captcha-element"></div>
      <button id="button">Verify</button>
      <div id="status">Click the button to start</div>
    </div>
    <script>
      (function () {
        const statusEl = document.getElementById('status');
        const setStatus = (msg) => {
          if (statusEl) statusEl.textContent = msg;
        };
        const submit = async (param) => {
          if (!param) {
            setStatus('Missing captcha result');
            return;
          }
          setStatus('Verified, submitting...');
          try {
            const resp = await fetch('/api/v1/captcha/manual/submit', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ verifyParam: param }),
              credentials: 'include',
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) {
              throw new Error(data.error || 'Submit failed');
            }
            setStatus('Submitted. You can close this tab.');
            setTimeout(() => {
              try {
                window.close();
              } catch (e) {}
            }, 800);
          } catch (err) {
            const msg = err && err.message ? err.message : 'Unknown error';
            setStatus('Submit failed: ' + msg);
          }
        };
        const init = () => {
          if (typeof window.initAliyunCaptcha !== 'function') {
            setStatus('Captcha script failed to load');
            return;
          }
          window.initAliyunCaptcha({
            SceneId: "{{.SceneID}}",
            mode: "popup",
            element: "#captcha-element",
            button: "#button",
            success: function (captchaVerifyParam) {
              submit(captchaVerifyParam);
            },
            fail: function () {
              setStatus('Verify failed, please retry');
            },
            rem: 1,
          });
        };
        if (document.readyState === 'loading') {
          document.addEventListener('DOMContentLoaded', init);
        } else {
          setTimeout(init, 0);
        }
      })();
    </script>
  </body>
</html>`))

func (s *Server) handleCaptchaManualPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cfg, err := fetchCaptchaManualConfig(ctx)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := captchaManualPageTpl.Execute(w, cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
}

type captchaManualSubmitPayload struct {
	VerifyParam string `json:"verifyParam"`
}

func (s *Server) handleCaptchaManualSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if s.engine == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "engine unavailable"})
		return
	}
	var body captchaManualSubmitPayload
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if strings.TrimSpace(body.VerifyParam) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "verifyParam is required"})
		return
	}
	if _, err := s.engine.AddCaptchaVerifyParamManual(body.VerifyParam); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"added": 1}})
}

func fetchCaptchaManualConfig(ctx context.Context) (captchaManualConfig, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, captchaManualSourceURL, nil)
	if err != nil {
		return captchaManualConfig{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return captchaManualConfig{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return captchaManualConfig{}, fmt.Errorf("upstream status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return captchaManualConfig{}, err
	}
	html := string(body)

	scene := matchFirst(html, captchaSceneIDRe)
	region := matchFirst(html, captchaRegionRe)
	prefix := matchFirst(html, captchaPrefixRe)
	if scene == "" || region == "" || prefix == "" {
		return captchaManualConfig{}, fmt.Errorf("failed to parse captcha config")
	}

	return captchaManualConfig{
		SceneID: scene,
		Region:  region,
		Prefix:  prefix,
	}, nil
}

func matchFirst(s string, re *regexp.Regexp) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}
