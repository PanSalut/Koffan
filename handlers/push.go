package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"shopping-list/db"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/gofiber/fiber/v2"
)

// vapidPrivateKey and vapidPublicKey hold the VAPID key pair used for Web Push.
// They are loaded/generated once at startup via InitVAPIDKeys.
var vapidPrivateKey string
var vapidPublicKey string

// InitVAPIDKeys loads VAPID keys from the database (or env vars as override),
// generating a fresh pair on first run and persisting them to the database.
func InitVAPIDKeys() {
	// Allow overriding keys via environment variables (useful for multi-instance deployments)
	if priv := os.Getenv("VAPID_PRIVATE_KEY"); priv != "" {
		vapidPrivateKey = priv
		if pub := os.Getenv("VAPID_PUBLIC_KEY"); pub != "" {
			vapidPublicKey = pub
		}
		log.Println("VAPID keys loaded from environment")
		return
	}

	// Try to load from database
	priv, privErr := db.GetAppSetting("vapid_private_key")
	pub, pubErr := db.GetAppSetting("vapid_public_key")

	if privErr == nil && pubErr == nil && priv != "" && pub != "" {
		vapidPrivateKey = priv
		vapidPublicKey = pub
		log.Println("VAPID keys loaded from database")
		return
	}

	// Generate a new key pair
	log.Println("Generating new VAPID key pair...")
	newPriv, newPub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		log.Printf("Failed to generate VAPID keys: %v", err)
		return
	}

	vapidPrivateKey = newPriv
	vapidPublicKey = newPub

	// Persist to database
	if err := db.SetAppSetting("vapid_private_key", vapidPrivateKey); err != nil {
		log.Printf("Failed to save VAPID private key: %v", err)
	}
	if err := db.SetAppSetting("vapid_public_key", vapidPublicKey); err != nil {
		log.Printf("Failed to save VAPID public key: %v", err)
	}

	log.Println("VAPID keys generated and stored")
}

// GetVAPIDPublicKey returns the server's VAPID public key so the client can subscribe.
func GetVAPIDPublicKey(c *fiber.Ctx) error {
	if vapidPublicKey == "" {
		return c.Status(503).JSON(fiber.Map{"error": "Push notifications not available"})
	}
	return c.JSON(fiber.Map{"public_key": vapidPublicKey})
}

// PushSubscribeRequest is the expected JSON body from the browser's PushSubscription.
type PushSubscribeRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		Auth   string `json:"auth"`
		P256dh string `json:"p256dh"`
	} `json:"keys"`
}

// SubscribePush registers (or refreshes) a browser push subscription.
func SubscribePush(c *fiber.Ctx) error {
	var req PushSubscribeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Endpoint == "" || req.Keys.Auth == "" || req.Keys.P256dh == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Missing required fields"})
	}

	if err := db.SavePushSubscription(req.Endpoint, req.Keys.Auth, req.Keys.P256dh); err != nil {
		log.Printf("Failed to save push subscription: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save subscription"})
	}

	return c.Status(201).JSON(fiber.Map{"status": "subscribed"})
}

// UnsubscribePush removes a browser push subscription.
func UnsubscribePush(c *fiber.Ctx) error {
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Endpoint == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Endpoint is required"})
	}

	if err := db.DeletePushSubscription(req.Endpoint); err != nil {
		log.Printf("Failed to delete push subscription: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to remove subscription"})
	}

	return c.JSON(fiber.Map{"status": "unsubscribed"})
}

// PushNotificationPayload is the JSON payload sent in a push notification.
type PushNotificationPayload struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	ListName string `json:"listName"`
	ItemName string `json:"itemName"`
	Event    string `json:"event"`
}

// SendPushNotifications sends a Web Push notification to all subscribers.
// It silently skips sending if no VAPID keys are available.
func SendPushNotifications(payload PushNotificationPayload) {
	if vapidPrivateKey == "" || vapidPublicKey == "" {
		return
	}

	subs, err := db.GetAllPushSubscriptions()
	if err != nil {
		log.Printf("Failed to retrieve push subscriptions: %v", err)
		return
	}
	if len(subs) == 0 {
		return
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal push payload: %v", err)
		return
	}

	for _, sub := range subs {
		go func(s db.PushSubscription) {
			subscription := &webpush.Subscription{
				Endpoint: s.Endpoint,
				Keys: webpush.Keys{
					Auth:   s.Auth,
					P256dh: s.P256dh,
				},
			}

			resp, err := webpush.SendNotification(payloadBytes, subscription, &webpush.Options{
				VAPIDPublicKey:  vapidPublicKey,
				VAPIDPrivateKey: vapidPrivateKey,
				TTL:             300,
			})
			if err != nil {
				log.Printf("Failed to send push notification to %s: %v", s.Endpoint, err)
				return
			}
			defer resp.Body.Close()

			// 410 Gone means the subscription is no longer valid — clean it up
			if resp.StatusCode == 410 {
				log.Printf("Push subscription expired (410), removing: %s", s.Endpoint)
				if delErr := db.DeletePushSubscription(s.Endpoint); delErr != nil && delErr != sql.ErrNoRows {
					log.Printf("Failed to remove expired push subscription: %v", delErr)
				}
			}
		}(sub)
	}
}
