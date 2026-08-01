package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	EventItemCreated   = "item.created"
	EventItemUpdated   = "item.updated"
	EventItemCompleted = "item.completed"
	EventItemDeleted   = "item.deleted"

	defaultTimeout = 5 * time.Second
	queueSize      = 256
)

var supportedEvents = map[string]struct{}{
	EventItemCreated:   {},
	EventItemUpdated:   {},
	EventItemCompleted: {},
	EventItemDeleted:   {},
}

// Config controls outbound webhook delivery.
type Config struct {
	URL        string
	Secret     string
	Events     string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// Event is the JSON envelope sent to the configured endpoint.
type Event struct {
	Event     string      `json:"event"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

type delivery struct {
	event string
	body  []byte
}

// Dispatcher validates, filters, queues, and delivers webhook events.
type Dispatcher struct {
	endpoint string
	secret   string
	events   map[string]struct{}
	client   *http.Client
	queue    chan delivery
}

var (
	defaultMu         sync.RWMutex
	defaultDispatcher = &Dispatcher{}
)

// New creates a dispatcher. An empty URL returns a disabled dispatcher.
func New(config Config) (*Dispatcher, error) {
	if strings.TrimSpace(config.URL) == "" {
		return &Dispatcher{}, nil
	}

	parsedURL, err := url.ParseRequestURI(config.URL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, fmt.Errorf("WEBHOOK_URL must be a valid HTTP or HTTPS URL")
	}

	events, err := parseEvents(config.Events)
	if err != nil {
		return nil, err
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	dispatcher := &Dispatcher{
		endpoint: parsedURL.String(),
		secret:   config.Secret,
		events:   events,
		client:   client,
		queue:    make(chan delivery, queueSize),
	}
	go dispatcher.run()
	return dispatcher, nil
}

// ConfigureFromEnv replaces the package dispatcher using WEBHOOK_URL,
// WEBHOOK_SECRET, and WEBHOOK_EVENTS.
func ConfigureFromEnv() error {
	dispatcher, err := New(Config{
		URL:    os.Getenv("WEBHOOK_URL"),
		Secret: os.Getenv("WEBHOOK_SECRET"),
		Events: os.Getenv("WEBHOOK_EVENTS"),
	})
	if err != nil {
		return err
	}

	defaultMu.Lock()
	defaultDispatcher = dispatcher
	defaultMu.Unlock()
	return nil
}

// Enabled reports whether this dispatcher has a configured endpoint.
func (dispatcher *Dispatcher) Enabled() bool {
	return dispatcher != nil && dispatcher.endpoint != ""
}

// Accepts reports whether a particular event would be queued.
func (dispatcher *Dispatcher) Accepts(event string) bool {
	if !dispatcher.Enabled() {
		return false
	}
	_, enabled := dispatcher.events[event]
	return enabled
}

// Notify queues an event without blocking the item mutation that triggered it.
// It returns false when delivery is disabled, filtered out, or the queue is full.
func (dispatcher *Dispatcher) Notify(event string, data interface{}) bool {
	if !dispatcher.Accepts(event) {
		return false
	}

	body, err := json.Marshal(Event{
		Event:     event,
		Timestamp: time.Now().UTC(),
		Data:      data,
	})
	if err != nil {
		log.Printf("Webhook event %s could not be encoded: %v", event, err)
		return false
	}

	select {
	case dispatcher.queue <- delivery{event: event, body: body}:
		return true
	default:
		log.Printf("Webhook queue is full, dropping event %s", event)
		return false
	}
}

// Notify queues an event on the package dispatcher.
func Notify(event string, data interface{}) bool {
	defaultMu.RLock()
	dispatcher := defaultDispatcher
	defaultMu.RUnlock()
	return dispatcher.Notify(event, data)
}

// Accepts reports whether the package dispatcher accepts an event.
func Accepts(event string) bool {
	defaultMu.RLock()
	dispatcher := defaultDispatcher
	defaultMu.RUnlock()
	return dispatcher.Accepts(event)
}

func parseEvents(value string) (map[string]struct{}, error) {
	if strings.TrimSpace(value) == "" {
		events := make(map[string]struct{}, len(supportedEvents))
		for event := range supportedEvents {
			events[event] = struct{}{}
		}
		return events, nil
	}

	events := make(map[string]struct{})
	for _, rawEvent := range strings.Split(value, ",") {
		event := strings.TrimSpace(rawEvent)
		if _, supported := supportedEvents[event]; !supported {
			return nil, fmt.Errorf("WEBHOOK_EVENTS contains unsupported event %q", event)
		}
		events[event] = struct{}{}
	}
	return events, nil
}

func (dispatcher *Dispatcher) run() {
	for queued := range dispatcher.queue {
		if err := dispatcher.deliver(queued); err != nil {
			log.Printf("Webhook delivery failed for %s: %v", queued.event, err)
		}
	}
}

func (dispatcher *Dispatcher) deliver(queued delivery) error {
	request, err := http.NewRequest(http.MethodPost, dispatcher.endpoint, bytes.NewReader(queued.body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "Koffan-Webhook/1.0")
	request.Header.Set("X-Koffan-Event", queued.event)
	if dispatcher.secret != "" {
		mac := hmac.New(sha256.New, []byte(dispatcher.secret))
		_, _ = mac.Write(queued.body)
		request.Header.Set("X-Koffan-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	response, err := dispatcher.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("endpoint returned HTTP %d", response.StatusCode)
	}
	return nil
}
