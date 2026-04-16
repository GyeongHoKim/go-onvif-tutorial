// 08-recording: ONVIF Recording Service — Profile G (Edge Storage)
//
// The Recording Service (trse/trc) is part of ONVIF Profile G, which
// defines how devices handle on-device (edge) storage and recording.
//
// Profile G concepts:
//   - Recording: A logical container for one or more tracks (video, audio,
//     metadata) stored on the device's local storage (SD card, NAS, etc.)
//   - RecordingJob: Defines what triggers recording (always-on, event-based,
//     scheduled) and which source (video stream) to record.
//   - Track: A single media stream within a recording (e.g., video track,
//     audio track, metadata track).
//
// Profile G vs. Profile S:
//   - Profile S: Camera streams to a separate NVR/VMS for recording.
//     The VMS handles storage, playback, and search.
//   - Profile G: Camera records locally to its own storage.
//     Useful for: standalone cameras, bandwidth-constrained sites,
//     redundant recording (record locally + stream to NVR).
//
// Important: Not all cameras support Profile G. It requires the device
// to have local storage capabilities (SD card slot, built-in storage).
// Budget cameras and NVT-only devices typically do NOT support this.
//
// Note: The use-go/onvif library does not include a dedicated recording
// package. This example sends raw SOAP requests and parses XML responses
// to demonstrate the Recording Service API.
//
// Run: go run ./08-recording/
package main

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	goonvif "github.com/use-go/onvif"

	"github.com/gyeongho/go-onvif-tutorial/internal/config"
)

// ─── Raw SOAP Request Types ─────────────────────────────────────────────
// Since use-go/onvif does not have a recording package, we define
// the SOAP request structs manually with correct XML namespace tags.

// GetRecordings returns all recordings on the device.
type GetRecordings struct {
	XMLName string `xml:"trc:GetRecordings"`
}

// GetRecordingJobs returns all recording jobs (active, idle, etc.)
type GetRecordingJobs struct {
	XMLName string `xml:"trc:GetRecordingJobs"`
}

// GetRecordingSummary returns a high-level summary of recordings.
type GetRecordingSummary struct {
	XMLName string `xml:"trc:GetRecordingSummary"`
}

// ─── SOAP Response Types ────────────────────────────────────────────────

type soapEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    soapBody `xml:"Body"`
}

type soapBody struct {
	GetRecordingsResponse     getRecordingsResp     `xml:"GetRecordingsResponse"`
	GetRecordingJobsResponse  getRecordingJobsResp  `xml:"GetRecordingJobsResponse"`
	GetRecordingSummaryResponse getRecordingSummaryResp `xml:"GetRecordingSummaryResponse"`
	Fault                     *soapFault            `xml:"Fault"`
}

type soapFault struct {
	Code   faultCode   `xml:"Code"`
	Reason faultReason `xml:"Reason"`
}

type faultCode struct {
	Value string `xml:"Value"`
}

type faultReason struct {
	Text string `xml:"Text"`
}

type getRecordingsResp struct {
	RecordingItem []recordingItem `xml:"RecordingItem"`
}

type recordingItem struct {
	RecordingToken string          `xml:"RecordingToken"`
	Configuration  recordingConfig `xml:"Configuration"`
	Tracks         tracks          `xml:"Tracks"`
}

type recordingConfig struct {
	Source      recordingSource `xml:"Source"`
	Content     string          `xml:"Content"`
	MaximumRetentionTime string `xml:"MaximumRetentionTime"`
}

type recordingSource struct {
	SourceId    string `xml:"SourceId"`
	Name        string `xml:"Name"`
	Location    string `xml:"Location"`
	Description string `xml:"Description"`
	Address     string `xml:"Address"`
}

type tracks struct {
	Track []track `xml:"Track"`
}

type track struct {
	TrackToken    string      `xml:"TrackToken"`
	Configuration trackConfig `xml:"Configuration"`
}

type trackConfig struct {
	TrackType   string `xml:"TrackType"`
	Description string `xml:"Description"`
}

type getRecordingJobsResp struct {
	JobItem []jobItem `xml:"JobItem"`
}

type jobItem struct {
	JobToken      string    `xml:"JobToken"`
	JobConfiguration jobConfig `xml:"JobConfiguration"`
}

type jobConfig struct {
	RecordingToken string  `xml:"RecordingToken"`
	Mode           string  `xml:"Mode"`
	Priority       int     `xml:"Priority"`
	Source         jobSource `xml:"Source"`
}

type jobSource struct {
	SourceToken    sourceToken    `xml:"SourceToken"`
	AutoCreateReceiver bool       `xml:"AutoCreateReceiver"`
}

type sourceToken struct {
	Token        string `xml:"Token"`
	Type         string `xml:"Type"`
}

type getRecordingSummaryResp struct {
	DataFrom        string `xml:"DataFrom"`
	DataUntil       string `xml:"DataUntil"`
	NumberRecordings int    `xml:"NumberRecordings"`
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

	fmt.Println("=== ONVIF Recording Service (Profile G) ===")
	fmt.Println()

	// Check if the device has a recording service endpoint.
	// The recording service is registered under "recording" or similar
	// keys in the device's service map.
	services := dev.GetServices()
	hasRecording := false
	for key := range services {
		if strings.Contains(strings.ToLower(key), "recording") ||
			strings.Contains(strings.ToLower(key), "search") {
			hasRecording = true
			fmt.Printf("  Found service: %s -> %s\n", key, services[key])
		}
	}

	if !hasRecording {
		fmt.Println("  WARNING: This device does not appear to have Recording Service endpoints.")
		fmt.Println("  Profile G (edge storage) requires the device to have local storage")
		fmt.Println("  capabilities (SD card, built-in NAS, etc.).")
		fmt.Println()
		fmt.Println("  The following commands will be attempted but may fail.")
		fmt.Println("  This is expected for Profile S-only cameras.")
	}
	fmt.Println()

	// ─── 1. GetRecordingSummary ─────────────────────────────────────────
	// Returns a high-level summary: date range of stored recordings
	// and total number of recordings. Quick way to check if the device
	// has any recorded content.
	fmt.Println("--- 1. Recording Summary ---")
	summaryResp, summaryErr := callAndParse(dev, GetRecordingSummary{})
	if summaryErr != nil {
		fmt.Printf("  GetRecordingSummary: %v\n", summaryErr)
		fmt.Println("  (This is expected if the device does not support Profile G)")
	} else {
		summary := summaryResp.Body.GetRecordingSummaryResponse
		if summary.NumberRecordings > 0 || summary.DataFrom != "" {
			fmt.Printf("  Data From        : %s\n", summary.DataFrom)
			fmt.Printf("  Data Until       : %s\n", summary.DataUntil)
			fmt.Printf("  Number Recordings: %d\n", summary.NumberRecordings)
		} else {
			fmt.Println("  No recordings found (device may not have any stored content)")
		}
	}
	fmt.Println()

	// ─── 2. GetRecordings ───────────────────────────────────────────────
	// Lists all recordings with their metadata: source info, tracks,
	// content description, and retention policy.
	//
	// Each recording has one or more tracks:
	//   - Video: The video stream (H.264, H.265, MJPEG)
	//   - Audio: The audio stream (G.711, AAC)
	//   - Metadata: Analytics metadata (motion regions, object tracking)
	fmt.Println("--- 2. Recordings ---")
	recResp, recErr := callAndParse(dev, GetRecordings{})
	if recErr != nil {
		fmt.Printf("  GetRecordings: %v\n", recErr)
	} else {
		recordings := recResp.Body.GetRecordingsResponse.RecordingItem
		if len(recordings) == 0 {
			fmt.Println("  No recordings found")
		}
		for i, rec := range recordings {
			fmt.Printf("\n  Recording #%d:\n", i+1)
			fmt.Printf("    Token      : %s\n", rec.RecordingToken)
			fmt.Printf("    Content    : %s\n", rec.Configuration.Content)
			fmt.Printf("    Retention  : %s\n", rec.Configuration.MaximumRetentionTime)

			src := rec.Configuration.Source
			if src.Name != "" {
				fmt.Printf("    Source Name: %s\n", src.Name)
			}
			if src.Location != "" {
				fmt.Printf("    Location   : %s\n", src.Location)
			}
			if src.Address != "" {
				fmt.Printf("    Address    : %s\n", src.Address)
			}

			if len(rec.Tracks.Track) > 0 {
				fmt.Println("    Tracks:")
				for _, t := range rec.Tracks.Track {
					fmt.Printf("      - %s (Token: %s, Desc: %s)\n",
						t.Configuration.TrackType, t.TrackToken, t.Configuration.Description)
				}
			}
		}
	}
	fmt.Println()

	// ─── 3. GetRecordingJobs ────────────────────────────────────────────
	// Lists recording jobs — these define WHEN and WHAT to record.
	//
	// Job modes:
	//   - "Idle": Job is defined but not actively recording
	//   - "Active": Job is currently recording
	//   - "Error": Job encountered an error (storage full, source unavailable)
	//
	// Recording triggers:
	//   - Always: Continuous recording 24/7
	//   - Event-based: Record only when motion is detected
	//   - Scheduled: Record during specific time windows
	fmt.Println("--- 3. Recording Jobs ---")
	jobsResp, jobsErr := callAndParse(dev, GetRecordingJobs{})
	if jobsErr != nil {
		fmt.Printf("  GetRecordingJobs: %v\n", jobsErr)
	} else {
		jobs := jobsResp.Body.GetRecordingJobsResponse.JobItem
		if len(jobs) == 0 {
			fmt.Println("  No recording jobs found")
		}
		for i, job := range jobs {
			fmt.Printf("\n  Job #%d:\n", i+1)
			fmt.Printf("    Token          : %s\n", job.JobToken)
			fmt.Printf("    Recording Token: %s\n", job.JobConfiguration.RecordingToken)
			fmt.Printf("    Mode           : %s\n", job.JobConfiguration.Mode)
			fmt.Printf("    Priority       : %d\n", job.JobConfiguration.Priority)
		}
	}
	fmt.Println()

	fmt.Println("=== Recording Service Exploration Complete ===")
	fmt.Println()
	fmt.Println("Notes:")
	fmt.Println("  - Profile G support depends on the device hardware")
	fmt.Println("  - Most IP cameras without SD card slots do NOT support Profile G")
	fmt.Println("  - NVR devices typically support Profile G for all connected cameras")
	fmt.Println("  - For Profile S cameras, recording is handled by the VMS/NVR software")
}

// callAndParse sends a raw SOAP request to the device and parses the
// XML response into our soapEnvelope struct.
func callAndParse(dev *goonvif.Device, method interface{}) (*soapEnvelope, error) {
	resp, err := dev.CallMethod(method)
	if err != nil {
		return nil, fmt.Errorf("call failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Try to extract SOAP fault message
		var env soapEnvelope
		if xml.Unmarshal(body, &env) == nil && env.Body.Fault != nil {
			return nil, fmt.Errorf("SOAP fault: %s - %s",
				env.Body.Fault.Code.Value, env.Body.Fault.Reason.Text)
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var env soapEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse failed: %w", err)
	}

	return &env, nil
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
