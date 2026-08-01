package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
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

	defaultTimeout      = 5 * time.Second
	defaultPollInterval = 250 * time.Millisecond
	maxRetryDelay       = 5 * time.Minute
)

var supportedEvents = map[string]struct{}{
	EventItemCreated:   {},
	EventItemUpdated:   {},
	EventItemCompleted: {},
	EventItemDeleted:   {},
}

// Config controls durable outbound webhook delivery.
type Config struct {
	URL          string
	Secret       string
	Events       string
	Timeout      time.Duration
	PollInterval time.Duration
	HTTPClient   *http.Client
	DB           *sql.DB
}

// Event is the JSON envelope sent to the configured endpoint.
type Event struct {
	ID        string      `json:"id"`
	Event     string      `json:"event"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

type delivery struct {
	id          int64
	event       string
	body        []byte
	attempts    int
	availableAt int64
}

// Dispatcher persists events in SQLite and retries them in order until they
// are delivered successfully.
type Dispatcher struct {
	endpoint     string
	secret       string
	events       map[string]struct{}
	client       *http.Client
	db           *sql.DB
	pollInterval time.Duration
	wake         chan struct{}
	stop         chan struct{}
	done         chan struct{}
	closeOnce    sync.Once
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
	if config.DB == nil {
		return nil, fmt.Errorf("webhook outbox database is required")
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
	pollInterval := config.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}

	dispatcher := &Dispatcher{
		endpoint:     parsedURL.String(),
		secret:       config.Secret,
		events:       events,
		client:       client,
		db:           config.DB,
		pollInterval: pollInterval,
		wake:         make(chan struct{}, 1),
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
	}
	go dispatcher.run()
	dispatcher.signal()
	return dispatcher, nil
}

// ConfigureFromEnv replaces the package dispatcher using WEBHOOK_URL,
// WEBHOOK_SECRET, and WEBHOOK_EVENTS.
func ConfigureFromEnv(database *sql.DB) error {
	dispatcher, err := New(Config{
		URL:    os.Getenv("WEBHOOK_URL"),
		Secret: os.Getenv("WEBHOOK_SECRET"),
		Events: os.Getenv("WEBHOOK_EVENTS"),
		DB:     database,
	})
	if err != nil {
		return err
	}

	defaultMu.Lock()
	previous := defaultDispatcher
	defaultDispatcher = dispatcher
	defaultMu.Unlock()

	if previous.Enabled() {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
		_ = previous.Close(ctx)
	}
	return nil
}

// Enabled reports whether this dispatcher has a configured endpoint.
func (dispatcher *Dispatcher) Enabled() bool {
	return dispatcher != nil && dispatcher.endpoint != ""
}

// Accepts reports whether a particular event will be persisted for delivery.
func (dispatcher *Dispatcher) Accepts(event string) bool {
	if !dispatcher.Enabled() {
		return false
	}
	_, enabled := dispatcher.events[event]
	return enabled
}

// Notify stores an event in the durable outbox. It returns only after SQLite
// accepts the event, while the outbound HTTP request remains asynchronous.
func (dispatcher *Dispatcher) Notify(event string, data interface{}) bool {
	if !dispatcher.Accepts(event) {
		return false
	}

	eventID, err := newEventID()
	if err != nil {
		log.Printf("Webhook event %s could not get an ID: %v", event, err)
		return false
	}

	body, err := json.Marshal(Event{
		ID:        eventID,
		Event:     event,
		Timestamp: time.Now().UTC(),
		Data:      data,
	})
	if err != nil {
		log.Printf("Webhook event %s could not be encoded: %v", event, err)
		return false
	}

	if _, err := dispatcher.db.Exec(`
		INSERT INTO webhook_outbox (event, payload) VALUES (?, ?)
	`, event, body); err != nil {
		log.Printf("Webhook event %s could not be persisted: %v", event, err)
		return false
	}

	dispatcher.signal()
	return true
}

func newEventID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

// Notify stores an event on the package dispatcher.
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

// Shutdown stops the package worker. Pending rows remain in SQLite and will be
// retried on the next application start.
func Shutdown(ctx context.Context) error {
	defaultMu.RLock()
	dispatcher := defaultDispatcher
	defaultMu.RUnlock()
	return dispatcher.Close(ctx)
}

// Close stops a dispatcher without deleting pending outbox rows.
func (dispatcher *Dispatcher) Close(ctx context.Context) error {
	if !dispatcher.Enabled() {
		return nil
	}
	dispatcher.closeOnce.Do(func() {
		close(dispatcher.stop)
	})
	select {
	case <-dispatcher.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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

func (dispatcher *Dispatcher) signal() {
	select {
	case dispatcher.wake <- struct{}{}:
	default:
	}
}

func (dispatcher *Dispatcher) run() {
	defer close(dispatcher.done)
	ticker := time.NewTicker(dispatcher.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-dispatcher.stop:
			return
		case <-dispatcher.wake:
			dispatcher.processAvailable()
		case <-ticker.C:
			dispatcher.processAvailable()
		}
	}
}

func (dispatcher *Dispatcher) processAvailable() {
	for {
		select {
		case <-dispatcher.stop:
			return
		default:
		}

		queued, err := dispatcher.nextDelivery()
		if err == sql.ErrNoRows {
			return
		}
		if err != nil {
			log.Printf("Webhook outbox could not be read: %v", err)
			return
		}
		if queued.availableAt > time.Now().Unix() {
			return
		}

		if err := dispatcher.deliver(queued); err != nil {
			dispatcher.scheduleRetry(queued, err)
			return
		}
		if _, err := dispatcher.db.Exec("DELETE FROM webhook_outbox WHERE id = ?", queued.id); err != nil {
			log.Printf("Delivered webhook %d could not be removed from outbox: %v", queued.id, err)
			return
		}
	}
}

func (dispatcher *Dispatcher) nextDelivery() (delivery, error) {
	var queued delivery
	err := dispatcher.db.QueryRow(`
		SELECT id, event, payload, attempts, available_at
		FROM webhook_outbox
		ORDER BY id
		LIMIT 1
	`).Scan(&queued.id, &queued.event, &queued.body, &queued.attempts, &queued.availableAt)
	return queued, err
}

func (dispatcher *Dispatcher) scheduleRetry(queued delivery, deliveryErr error) {
	delay := time.Second << min(queued.attempts, 8)
	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}
	message := deliveryErr.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	_, err := dispatcher.db.Exec(`
		UPDATE webhook_outbox
		SET attempts = attempts + 1, available_at = ?, last_error = ?
		WHERE id = ?
	`, time.Now().Add(delay).Unix(), message, queued.id)
	if err != nil {
		log.Printf("Webhook retry could not be scheduled for %s: %v", queued.event, err)
		return
	}
	log.Printf("Webhook delivery failed for %s, retrying in %s: %v", queued.event, delay, deliveryErr)
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
