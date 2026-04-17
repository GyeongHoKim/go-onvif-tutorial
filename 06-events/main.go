// 06-events: ONVIF Event Service — PullPoint Subscriptions
//
// The Event Service (tev) enables cameras to notify clients about
// real-time events such as:
//   - Motion detection (VideoAnalytics/MotionDetection)
//   - Camera tampering (VideoSource/Tampering)
//   - Digital input changes (Device/Trigger/DigitalInput)
//   - Storage failure, network loss, etc.
//
// ONVIF supports two event delivery mechanisms:
//
//  1. PullPoint (used here): The client creates a subscription, then
//     periodically "pulls" events from the camera. This is simpler
//     because it works through NAT/firewalls — the client initiates
//     all connections. Most VMS software uses this approach.
//
//  2. WS-BaseNotification (push): The camera pushes events to a
//     client-hosted HTTP endpoint. Requires the client to run an HTTP
//     server that the camera can reach — problematic with NAT/firewalls.
//
// PullPoint workflow:
//  1. CreatePullPointSubscription — camera creates a subscription and
//     returns a subscription endpoint URL.
//  2. PullMessages (loop) — client polls the subscription endpoint.
//     The camera blocks the response until events occur or timeout.
//     This is similar to HTTP long polling.
//  3. Unsubscribe — clean up when done (important to free camera resources).
//
// Run: go run ./06-events/
package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	goonvif "github.com/use-go/onvif"
	"github.com/use-go/onvif/event"

	"github.com/gyeongho/go-onvif-tutorial/internal/config"
)

// soapEnvelope is a generic SOAP envelope for parsing responses.
// The use-go/onvif library's event package doesn't have SDK wrappers,
// so we parse the SOAP XML responses manually.
type soapEnvelope struct {
	XMLName xml.Name    `xml:"Envelope"`
	Body    soapBodyMsg `xml:"Body"`
}

type soapBodyMsg struct {
	CreateResponse createPullPointResponse `xml:"CreatePullPointSubscriptionResponse"`
	PullResponse   pullMessagesResponse    `xml:"PullMessagesResponse"`
	Fault          soapFault               `xml:"Fault"`
}

type soapFault struct {
	Code   soapFaultCode   `xml:"Code"`
	Reason soapFaultReason `xml:"Reason"`
}

type soapFaultCode struct {
	Value   string `xml:"Value"`
	Subcode string `xml:"Subcode>Value"`
}

type soapFaultReason struct {
	Text string `xml:"Text"`
}

type createPullPointResponse struct {
	SubscriptionReference subscriptionRef `xml:"SubscriptionReference"`
	CurrentTime           string          `xml:"CurrentTime"`
	TerminationTime       string          `xml:"TerminationTime"`
}

type subscriptionRef struct {
	Address string `xml:"Address"`
}

type pullMessagesResponse struct {
	CurrentTime         string                `xml:"CurrentTime"`
	TerminationTime     string                `xml:"TerminationTime"`
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

// topicNode is one node in the topic tree returned by GetEventProperties.
type topicNode struct {
	Name     string
	Children []*topicNode
}

// schemaNodes are element names that describe message structure, not topics.
// We skip them when printing the topic tree.
var schemaNodes = map[string]bool{
	"MessageDescription":     true,
	"Source":                 true,
	"Data":                   true,
	"SimpleItemDescription":  true,
	"ElementItemDescription": true,
}

// parseTopicChildren reads child elements from decoder until the matching
// EndElement and builds a slice of topicNodes.
func parseTopicChildren(decoder *xml.Decoder) ([]*topicNode, error) {
	var nodes []*topicNode
	for {
		tok, err := decoder.Token()
		if err != nil {
			return nodes, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			children, err := parseTopicChildren(decoder)
			node := &topicNode{Name: t.Name.Local, Children: children}
			nodes = append(nodes, node)
			if err != nil {
				return nodes, err
			}
		case xml.EndElement:
			return nodes, nil
		}
	}
}

// printTopicTree prints the topic tree with indentation, skipping schema nodes.
func printTopicTree(nodes []*topicNode, indent int) {
	for _, n := range nodes {
		if schemaNodes[n.Name] {
			continue
		}
		fmt.Printf("%s%s\n", strings.Repeat("  ", indent), n.Name)
		printTopicTree(n.Children, indent+1)
	}
}

// printSupportedTopics calls GetEventProperties and prints the topic tree.
func printSupportedTopics(dev interface {
	CallMethod(interface{}) (*http.Response, error)
}) error {
	resp, err := dev.CallMethod(event.GetEventProperties{})
	if err != nil {
		return fmt.Errorf("GetEventProperties: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
	}

	decoder := xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "TopicSet" {
			nodes, _ := parseTopicChildren(decoder)
			printTopicTree(nodes, 0)
			return nil
		}
	}
	fmt.Println("  (no TopicSet found in response)")
	return nil
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	dev, err := goonvif.NewDevice(goonvif.DeviceParams{
		Xaddr:    cfg.Xaddr(),
		Username: cfg.Username,
		Password: cfg.Password,
	})
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}

	fmt.Println("=== ONVIF Event Service (PullPoint) ===")
	fmt.Println()

	// ─── 0. GetEventProperties — supported topics ────────────────────────
	fmt.Println("--- Supported Event Topics ---")
	if err := printSupportedTopics(dev); err != nil {
		fmt.Printf("  (could not retrieve topics: %v)\n", err)
	}
	fmt.Println()

	// ─── 1. CreatePullPointSubscription ─────────────────────────────────
	// This tells the camera to start buffering events for us.
	// The camera returns a subscription reference URL that we'll use
	// for pulling messages.
	//
	// InitialTerminationTime sets how long the subscription lives.
	// If we don't renew it (via Renew), it auto-expires to free
	// camera resources. PT60S = 60 seconds in ISO 8601 duration format.
	//
	// NOTE: The use-go/onvif library's CreatePullPointSubscription struct has
	// a typo in the SubscriptionPolicy XML tag ("wsnt:sSubscriptionPolicy"
	// instead of "wsnt:SubscriptionPolicy"), which causes cameras to reject
	// the request with a SOAP Sender fault.
	// See: https://github.com/use-go/onvif/issues/63
	//
	// Once that issue is fixed, this can be simplified to:
	//
	//   createReq := event.CreatePullPointSubscription{}
	//   resp, err := dev.CallMethod(createReq)
	//
	// Until then, we send the SOAP request manually.
	fmt.Println("Creating PullPoint subscription...")

	eventEndpoint := dev.GetEndpoint("events")
	if eventEndpoint == "" {
		log.Fatal("Event service endpoint not found on this device")
	}

	const createPullPointSOAP = `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tev="http://www.onvif.org/ver10/events/wsdl"
            xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
  <s:Body>
    <tev:CreatePullPointSubscription>
      <wsnt:InitialTerminationTime>PT60S</wsnt:InitialTerminationTime>
    </tev:CreatePullPointSubscription>
  </s:Body>
</s:Envelope>`

	httpResp, err := http.Post(eventEndpoint, "application/soap+xml; charset=utf-8", strings.NewReader(createPullPointSOAP))
	if err != nil {
		log.Fatalf("CreatePullPointSubscription failed: %v", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		log.Fatalf("Failed to read response: %v", err)
	}

	var createEnv soapEnvelope
	if err := xml.Unmarshal(body, &createEnv); err != nil {
		log.Fatalf("Failed to parse CreatePullPointSubscription response: %v", err)
	}

	if f := createEnv.Body.Fault; f.Code.Value != "" {
		log.Fatalf("CreatePullPointSubscription SOAP Fault:\n  Code    : %s\n  Subcode : %s\n  Reason  : %s\n  Raw XML :\n%s",
			f.Code.Value, f.Code.Subcode, f.Reason.Text, string(body))
	}

	subAddr := createEnv.Body.CreateResponse.SubscriptionReference.Address
	fmt.Printf("  Subscription created!\n")
	fmt.Printf("  Endpoint       : %s\n", subAddr)
	fmt.Printf("  Termination    : %s\n", createEnv.Body.CreateResponse.TerminationTime)
	fmt.Println()

	// ─── 2. PullMessages Loop ───────────────────────────────────────────
	// Now we continuously pull events from the subscription endpoint.
	// PullMessages blocks on the camera side until either:
	//   - One or more events occur, OR
	//   - The specified Timeout expires
	//
	// This is similar to HTTP long-polling and is very efficient —
	// no wasted bandwidth when there are no events.
	fmt.Println("Listening for events... (press Ctrl+C to stop)")
	fmt.Println("Trigger motion in front of the camera to see events.")
	fmt.Println()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	stopChan := make(chan struct{})
	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		close(stopChan)
	}()

	pollCount := 0
	eventCount := 0

	for {
		select {
		case <-stopChan:
			goto cleanup
		default:
		}

		pollCount++

		pullReq := event.PullMessages{
			Timeout:      "PT5S",
			MessageLimit: 100,
		}

		pullResp, err := dev.CallMethod(pullReq)
		if err != nil {
			log.Printf("[Poll #%d] PullMessages failed: %v", pollCount, err)
			time.Sleep(2 * time.Second)
			continue
		}

		pullBody, _ := io.ReadAll(pullResp.Body)
		pullResp.Body.Close()

		if pullResp.StatusCode != http.StatusOK {
			log.Printf("[Poll #%d] HTTP %d", pollCount, pullResp.StatusCode)
			time.Sleep(2 * time.Second)
			continue
		}

		var pullEnv soapEnvelope
		if err := xml.Unmarshal(pullBody, &pullEnv); err != nil {
			log.Printf("[Poll #%d] Parse error: %v", pollCount, err)
			continue
		}

		msgs := pullEnv.Body.PullResponse.NotificationMessage
		if len(msgs) == 0 {
			fmt.Printf("[Poll #%d] No events (waiting...)\n", pollCount)
			continue
		}

		for _, msg := range msgs {
			eventCount++
			timestamp := msg.Message.Inner.UtcTime
			if timestamp == "" {
				timestamp = time.Now().UTC().Format(time.RFC3339)
			}

			fmt.Printf("[Event #%d] %s\n", eventCount, timestamp)
			fmt.Printf("  Topic  : %s\n", msg.Topic.Value)

			if len(msg.Message.Inner.Source.Items) > 0 {
				fmt.Printf("  Source :")
				for _, item := range msg.Message.Inner.Source.Items {
					fmt.Printf(" %s=%s", item.Name, item.Value)
				}
				fmt.Println()
			}

			if len(msg.Message.Inner.Data.Items) > 0 {
				fmt.Printf("  Data   :")
				for _, item := range msg.Message.Inner.Data.Items {
					fmt.Printf(" %s=%s", item.Name, item.Value)
				}
				fmt.Println()
			}
			fmt.Println()
		}
	}

cleanup:
	// ─── 3. Unsubscribe ─────────────────────────────────────────────────
	// Always unsubscribe when done to free camera resources.
	// Cameras have a limited number of concurrent subscriptions
	// (typically 5-10). Leaked subscriptions can prevent new clients
	// from receiving events until they auto-expire.
	fmt.Println("Unsubscribing...")
	unsubResp, err := dev.CallMethod(event.Unsubscribe{})
	if err != nil {
		log.Printf("Unsubscribe failed: %v (subscription will auto-expire)", err)
	} else {
		unsubResp.Body.Close()
		fmt.Println("  Unsubscribed successfully")
	}

	fmt.Printf("\nSummary: %d polls, %d events received\n", pollCount, eventCount)
}
