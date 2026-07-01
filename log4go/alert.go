package log4go

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// AlertLevel is the severity of an alert.
type AlertLevel int

const (
	AlertInfo AlertLevel = iota
	AlertWarn
	AlertError
)

func (l AlertLevel) String() string {
	switch l {
	case AlertWarn:
		return "WARN"
	case AlertError:
		return "ERROR"
	default:
		return "INFO"
	}
}

// AlertSink receives overflow/health alerts, decoupled from the log4go log path
// (uses the standard logger / network, never log4go itself, to avoid recursion).
// Implementations: LogAlertSink (default), WebhookAlertSink (lark/dingtalk/feishu),
// or your own (Prometheus Alertmanager, email, ...).
type AlertSink interface {
	Send(level AlertLevel, kind, text string)
	Close() error
}

// LogAlertSink writes alerts via the standard logger.
type LogAlertSink struct{}

// Send logs the alert.
func (LogAlertSink) Send(level AlertLevel, kind, text string) {
	log.Printf("[log4go] alert %s %s: %s", level, kind, text)
}

// Close is a no-op.
func (LogAlertSink) Close() error { return nil }

// AlertFormatter formats a webhook payload for a target OA platform.
// Returns the Content-Type and the request body.
type AlertFormatter func(level AlertLevel, kind, text string) (contentType string, body []byte)

// LarkTextFormatter formats a Lark/Feishu text post.
func LarkTextFormatter(_ string) AlertFormatter {
	return func(level AlertLevel, kind, text string) (string, []byte) {
		msg := fmt.Sprintf("[log4go] %s %s: %s", level, kind, text)
		return "application/json", []byte(fmt.Sprintf(`{"msg_type":"text","content":{"text":%q}}`, msg))
	}
}

// DingtalkTextFormatter formats a DingTalk text post.
func DingtalkTextFormatter(_ string) AlertFormatter {
	return func(level AlertLevel, kind, text string) (string, []byte) {
		msg := fmt.Sprintf("[log4go] %s %s: %s", level, kind, text)
		return "application/json", []byte(fmt.Sprintf(`{"msgtype":"text","text":{"content":%q}}`, msg))
	}
}

// WechatTextFormatter formats a WeCom (企业微信) text post.
func WechatTextFormatter(_ string) AlertFormatter {
	return func(level AlertLevel, kind, text string) (string, []byte) {
		msg := fmt.Sprintf("[log4go] %s %s: %s", level, kind, text)
		return "application/json", []byte(fmt.Sprintf(`{"msgtype":"text","text":{"content":%q}}`, msg))
	}
}

// WebhookAlertSink POSTs alerts to a webhook URL asynchronously. Send is
// non-blocking and bounded (drops on full queue) so the log path is never
// blocked and the sink cannot cause OOM.
type WebhookAlertSink struct {
	url        string
	client     *http.Client
	formatter  AlertFormatter
	ch         chan alertMsg
	once       sync.Once
	quit       chan struct{}
	maxRetries int
	maxPerSec  int
	rmux       sync.Mutex
	rCount     int
	rWindow    time.Time
}

type alertMsg struct {
	level AlertLevel
	kind  string
	text  string
}

// NewWebhookAlertSink creates an async webhook sink. queueSize bounds the async
// queue (<=0 -> 256). formatter formats the payload; nil -> Lark text.
func NewWebhookAlertSink(url string, queueSize int, formatter AlertFormatter) *WebhookAlertSink {
	if queueSize <= 0 {
		queueSize = 256
	}
	if formatter == nil {
		formatter = LarkTextFormatter(url)
	}
	w := &WebhookAlertSink{
		url:       url,
		client:    &http.Client{Timeout: 3 * time.Second},
		formatter: formatter,
		ch:        make(chan alertMsg, queueSize),
		quit:      make(chan struct{}),
	}
	go w.daemon()
	return w
}

// Send enqueues an alert (non-blocking; drops on full queue or when rate-limited).
func (w *WebhookAlertSink) Send(level AlertLevel, kind, text string) {
	if !w.allow() {
		return
	}
	select {
	case w.ch <- alertMsg{level: level, kind: kind, text: text}:
	default:
	}
}

// SetRateLimit caps alerts per second (0 = unlimited). Protects OA webhooks
// from flooding during sustained overflow.
func (w *WebhookAlertSink) SetRateLimit(perSec int) { w.maxPerSec = perSec }

func (w *WebhookAlertSink) allow() bool {
	if w.maxPerSec <= 0 {
		return true
	}
	w.rmux.Lock()
	defer w.rmux.Unlock()
	now := time.Now()
	if now.Sub(w.rWindow) >= time.Second {
		w.rWindow = now
		w.rCount = 0
	}
	if w.rCount >= w.maxPerSec {
		return false
	}
	w.rCount++
	return true
}

// SetMaxRetries sets how many times a failed POST is retried (default 0).
func (w *WebhookAlertSink) SetMaxRetries(n int) { w.maxRetries = n }

func (w *WebhookAlertSink) daemon() {
	defer func() {
		if r := recover(); r != nil {
			recordDaemonPanic("webhook", r)
		}
	}()
	for {
		select {
		case m := <-w.ch:
			ct, body := w.formatter(m.level, m.kind, m.text)
			for attempt := 0; ; attempt++ {
				req, err := http.NewRequest(http.MethodPost, w.url, bytes.NewReader(body)) //nolint:gosec // user-configured URL
				if err != nil {
					break
				}
				req.Header.Set("Content-Type", ct)
				resp, doErr := w.client.Do(req)
				if doErr == nil {
					_ = resp.Body.Close()
					if resp.StatusCode < 500 {
						break
					}
				}
				if attempt >= w.maxRetries {
					break
				}
				time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
			}
		case <-w.quit:
			return
		}
	}
}

// Close stops the daemon (pending queued alerts may be dropped).
func (w *WebhookAlertSink) Close() error {
	w.once.Do(func() { close(w.quit) })
	return nil
}
