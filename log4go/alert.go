package log4go

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// AlertLevel is the severity of an alert.
type AlertLevel int

// AlertInfo / AlertWarn / AlertError are the alert severities surfaced to an
// AlertSink (drop is reported as error, spill as warn).
const (
	AlertInfo AlertLevel = iota
	AlertWarn
	AlertError
)

// String returns the uppercase severity name ("INFO"/"WARN"/"ERROR").
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
//
// The hot-path config (maxPerSec/maxRetries) is held in atomics so the rate
// limit and retry budget can be reconfigured live without racing the daemon:
// SetRateLimit/SetMaxRetries are concurrent-safe with Send and the daemon loop.
type WebhookAlertSink struct {
	url       string
	client    *http.Client
	formatter AlertFormatter
	ch        chan alertMsg
	once      sync.Once
	quit      chan struct{}
	wg        sync.WaitGroup
	// maxRetries/maxPerSec are read on the hot path (allow()) and by the daemon;
	// held in atomics so SetMaxRetries/SetRateLimit never race a concurrent Send.
	maxRetries atomic.Int64
	maxPerSec  atomic.Int64
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
	w.wg.Add(1)
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
// from flooding during sustained overflow. Safe to call concurrently with Send.
func (w *WebhookAlertSink) SetRateLimit(perSec int) { w.maxPerSec.Store(int64(perSec)) }

func (w *WebhookAlertSink) allow() bool {
	if w.maxPerSec.Load() <= 0 {
		return true
	}
	w.rmux.Lock()
	defer w.rmux.Unlock()
	now := time.Now()
	if now.Sub(w.rWindow) >= time.Second {
		w.rWindow = now
		w.rCount = 0
	}
	if w.rCount >= int(w.maxPerSec.Load()) {
		return false
	}
	w.rCount++
	return true
}

// SetMaxRetries sets how many times a failed POST is retried (default 0). Safe
// to call concurrently with Send and the daemon's retry loop.
func (w *WebhookAlertSink) SetMaxRetries(n int) { w.maxRetries.Store(int64(n)) }

func (w *WebhookAlertSink) daemon() {
	defer w.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			recordDaemonPanic("webhook", r)
		}
	}()
	for {
		select {
		case m := <-w.ch:
			w.post(m)
		case <-w.quit:
			// L7: don't silently drop queued alerts. Drain whatever is already
			// buffered (non-blocking) and attempt delivery before exiting, so a
			// graceful Close does not lose alerts enqueued before quit.
			w.drainQueue()
			return
		}
	}
}

// drainQueue attempts delivery of all alerts buffered in w.ch at the moment of
// shutdown, without blocking. Producers racing this drain simply see a full
// queue and drop (Send's default branch), which is acceptable.
func (w *WebhookAlertSink) drainQueue() {
	for {
		select {
		case m := <-w.ch:
			w.post(m)
		default:
			return
		}
	}
}

// post delivers a single alert with bounded retry. Each retry wait listens on
// w.quit so Close() interrupts an in-flight retry promptly instead of blocking
// for up to maxRetries*3.2s.
func (w *WebhookAlertSink) post(m alertMsg) {
	ct, body := w.formatter(m.level, m.kind, m.text)
	maxR := int(w.maxRetries.Load())
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequest(http.MethodPost, w.url, bytes.NewReader(body)) //nolint:gosec // user-configured URL
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", ct)
		resp, doErr := w.client.Do(req)
		if doErr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				return
			}
		}
		if attempt >= maxR {
			return
		}
		// Bounded exponential backoff that is also interruptible by Close:
		// without the quit case, Close() would have to wait up to ~3.2s per
		// remaining attempt before the daemon noticed shutdown.
		select {
		case <-time.After(time.Duration(attempt+1) * 200 * time.Millisecond):
		case <-w.quit:
			return
		}
	}
}

// Close stops the daemon. It interrupts any in-flight retry promptly and waits
// for the daemon to finish draining already-queued alerts (those buffered before
// Close) before returning, so there are no send-on-closed-channel panics and no
// stray goroutine outliving the sink.
func (w *WebhookAlertSink) Close() error {
	w.once.Do(func() {
		close(w.quit)
		w.wg.Wait()
	})
	return nil
}
