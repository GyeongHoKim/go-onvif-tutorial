// 09-real-world/stream-monitor: Multi-Camera Event Stream Monitor
//
// This example demonstrates real-time event monitoring across multiple
// ONVIF cameras — a core feature of any video management system (VMS).
//
// What this does:
//   - Connects to all configured cameras concurrently
//   - Creates a PullPoint event subscription on each camera
//   - Continuously polls for events (motion, tampering, digital inputs)
//   - Prints real-time alerts with timestamps and camera identification
//   - Handles graceful shutdown with Ctrl+C (unsubscribes from all cameras)
//
// In a real VMS, events trigger actions:
//   - Motion detected -> Start recording, send push notification
//   - Tampering detected -> Sound alarm, alert security personnel
//   - Digital input change -> Log access control event
//   - Camera offline -> Alert maintenance team
//
// Event types to watch for:
//   - tns1:VideoAnalytics/tnsaxis:MotionDetection (Axis cameras)
//   - tns1:RuleEngine/CellMotionDetector/Motion (ONVIF analytics)
//   - tns1:VideoSource/MotionAlarm (generic motion)
//   - tns1:Device/Trigger/DigitalInput (door sensor, relay)
//   - tns1:VideoSource/ImageTooBlurry (tampering)
//
// Run: go run ./09-real-world/stream-monitor/
package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	goonvif "github.com/use-go/onvif"
	"github.com/use-go/onvif/event"

	"github.com/gyeongho/go-onvif-tutorial/internal/config"
)

// ─── SOAP XML Parsing Types ─────────────────────────────────────────────

type soapEnvelope struct {
	XMLName xml.Name    `xml:"Envelope"`
	Body    soapBodyMsg `xml:"Body"`
}

type soapBodyMsg struct {
	CreateResponse createPullPointResponse `xml:"CreatePullPointSubscriptionResponse"`
	PullResponse   pullMessagesResponse    `xml:"PullMessagesResponse"`
}

type createPullPointResponse struct {
	SubscriptionReference subscriptionRef `xml:"SubscriptionReference"`
}

type subscriptionRef struct {
	Address string `xml:"Address"`
}

type pullMessagesResponse struct {
	NotificationMessage []notificationMessage `xml:"NotificationMessage"`
}

type notificationMessage struct {
	Topic   topicValue `xml:"Topic"`
	Message msgPayload `xml:"Message"`
}

type topicValue struct {
	Value string `xml:",chardata"`
}

type msgPayload struct {
	Inner innerMessage `xml:"Message"`
}

type innerMessage struct {
	UtcTime string      `xml:"UtcTime,attr"`
	Source  simpleItems `xml:"Source"`
	Data    simpleItems `xml:"Data"`
}

type simpleItems struct {
	Items []simpleItem `xml:"SimpleItem"`
}

type simpleItem struct {
	Name  string `xml:"Name,attr"`
	Value string `xml:"Value,attr"`
}

// Alert represents a single event received from a camera.
// In a real VMS, these would be stored in a database, forwarded
// to notification services, and displayed on a live event panel.
type Alert struct {
	Timestamp  time.Time
	CameraAddr string
	EventType  string
	Source     map[string]string
	Data       map[string]string
}

func main() {
	fmt.Println("=== Multi-Camera Event Stream Monitor ===")
	fmt.Println()

	cameras, err := config.LoadCameras()
	if err != nil {
		log.Fatalf("Failed to load camera config: %v", err)
	}

	fmt.Printf("Configured cameras: %d\n", len(cameras))
	fmt.Println()

	// Set up graceful shutdown with Ctrl+C
	ctx, cancel := createShutdownContext()
	defer cancel()

	// Channel for collecting alerts from all cameras
	alertChan := make(chan Alert, 100)

	// Track active goroutines
	var wg sync.WaitGroup

	// ─── Connect and subscribe to events on each camera ─────────────────
	for _, cam := range cameras {
		wg.Add(1)
		go func(cam *config.CameraConfig) {
			defer wg.Done()
			monitorCamera(ctx, cam, alertChan)
		}(cam)
	}

	// ─── Print events as they arrive ────────────────────────────────────
	// This goroutine reads from the alert channel and prints formatted
	// event output. In a real VMS, this would write to a database,
	// trigger recording, send push notifications, etc.
	go func() {
		for alert := range alertChan {
			printAlert(alert)
		}
	}()

	fmt.Println("Monitoring events... Press Ctrl+C to stop.")
	fmt.Println(strings.Repeat("-", 70))
	fmt.Println()

	// Wait for all camera monitors to finish (triggered by Ctrl+C)
	wg.Wait()
	close(alertChan)

	fmt.Println()
	fmt.Println("=== Monitor stopped ===")
}

// monitorCamera connects to a single camera, creates a PullPoint
// subscription, and continuously pulls events until the context is canceled.
func monitorCamera(ctx <-chan struct{}, cam *config.CameraConfig, alerts chan<- Alert) {
	xaddr := cam.Xaddr()
	log.Printf("[%s] Connecting...", xaddr)

	dev, err := goonvif.NewDevice(goonvif.DeviceParams{
		Xaddr:    xaddr,
		Username: cam.Username,
		Password: cam.Password,
	})
	if err != nil {
		log.Printf("[%s] Connection failed: %v", xaddr, err)
		return
	}
	log.Printf("[%s] Connected", xaddr)

	// Create PullPoint subscription
	createReq := event.CreatePullPointSubscription{}
	resp, err := dev.CallMethod(createReq)
	if err != nil {
		log.Printf("[%s] CreatePullPointSubscription failed: %v", xaddr, err)
		return
	}

	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[%s] CreatePullPointSubscription HTTP %d", xaddr, resp.StatusCode)
		return
	}

	var createEnv soapEnvelope
	if err := xml.Unmarshal(body, &createEnv); err != nil {
		log.Printf("[%s] Failed to parse subscription response: %v", xaddr, err)
		return
	}

	log.Printf("[%s] Subscription created: %s",
		xaddr, createEnv.Body.CreateResponse.SubscriptionReference.Address)

	// ─── PullMessages Loop ──────────────────────────────────────────────
	// Continuously poll for events. The camera will hold the connection
	// open (long-poll) until events occur or the timeout expires.
	for {
		select {
		case <-ctx:
			// Graceful shutdown: unsubscribe
			log.Printf("[%s] Unsubscribing...", xaddr)
			unsubReq := event.Unsubscribe{}
			unsubResp, err := dev.CallMethod(unsubReq)
			if err == nil {
				unsubResp.Body.Close()
			}
			log.Printf("[%s] Stopped", xaddr)
			return
		default:
		}

		pullReq := event.PullMessages{
			Timeout:      "PT5S",
			MessageLimit: 100,
		}

		pullResp, err := dev.CallMethod(pullReq)
		if err != nil {
			log.Printf("[%s] PullMessages failed: %v", xaddr, err)
			// Brief pause before retry to avoid tight error loop
			select {
			case <-ctx:
				return
			case <-time.After(3 * time.Second):
			}
			continue
		}

		pullBody, _ := io.ReadAll(pullResp.Body)
		pullResp.Body.Close()

		if pullResp.StatusCode != http.StatusOK {
			continue
		}

		var pullEnv soapEnvelope
		if err := xml.Unmarshal(pullBody, &pullEnv); err != nil {
			continue
		}

		// Process each received event
		for _, msg := range pullEnv.Body.PullResponse.NotificationMessage {
			alert := Alert{
				Timestamp:  time.Now().UTC(),
				CameraAddr: xaddr,
				EventType:  strings.TrimSpace(msg.Topic.Value),
				Source:     make(map[string]string),
				Data:       make(map[string]string),
			}

			if msg.Message.Inner.UtcTime != "" {
				if t, err := time.Parse(time.RFC3339, msg.Message.Inner.UtcTime); err == nil {
					alert.Timestamp = t
				}
			}

			for _, item := range msg.Message.Inner.Source.Items {
				alert.Source[item.Name] = item.Value
			}
			for _, item := range msg.Message.Inner.Data.Items {
				alert.Data[item.Name] = item.Value
			}

			alerts <- alert
		}
	}
}

// printAlert formats and prints a single event alert to stdout.
// The format mimics a syslog-style event log that operations teams
// are familiar with.
func printAlert(a Alert) {
	ts := a.Timestamp.Format("2006-01-02 15:04:05")

	// Extract the last part of the topic for a short event name
	shortTopic := a.EventType
	parts := strings.Split(a.EventType, "/")
	if len(parts) > 0 {
		shortTopic = parts[len(parts)-1]
	}

	fmt.Printf("[%s] [%s] %s", ts, a.CameraAddr, shortTopic)

	// Print key data fields inline
	for k, v := range a.Data {
		fmt.Printf(" | %s=%s", k, v)
	}
	for k, v := range a.Source {
		fmt.Printf(" | %s=%s", k, v)
	}
	fmt.Println()
}

// createShutdownContext returns a channel that is closed when Ctrl+C
// is pressed (SIGINT/SIGTERM). This is used to signal all goroutines
// to stop gracefully.
func createShutdownContext() (<-chan struct{}, func()) {
	done := make(chan struct{})
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nReceived shutdown signal...")
		close(done)
	}()

	cancel := func() {
		signal.Stop(sigChan)
	}

	return done, cancel
}
