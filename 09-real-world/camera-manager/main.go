// 09-real-world/camera-manager: Multi-Camera Concurrent Manager
//
// This example demonstrates how a real VMS connects to and manages
// multiple ONVIF cameras simultaneously — the fundamental pattern
// behind camera management in commercial video management systems.
//
// Key patterns demonstrated:
//   1. Concurrent camera connections using goroutines and sync.WaitGroup
//   2. Graceful handling of offline/unreachable cameras
//   3. Aggregating device info and stream URLs from multiple sources
//   4. Tabular output using tabwriter (like a VMS dashboard)
//
// How this scales in production:
//   - A VMS manages hundreds or thousands of cameras.
//   - Connection and status polling happen concurrently to avoid
//     one slow/offline camera blocking the entire system.
//   - Camera state (online/offline/error) is tracked in a database.
//   - A worker pool limits concurrent connections to avoid overwhelming
//     the network or the VMS server.
//
// Configuration:
//   Set CAMERA_1_HOST, CAMERA_2_HOST, CAMERA_3_HOST in .env.local
//   to test with multiple cameras. Cameras that are offline will be
//   reported as errors without blocking others.
//
// Run: go run ./09-real-world/camera-manager/
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"text/tabwriter"
	"time"

	goonvif "github.com/use-go/onvif"
	"github.com/use-go/onvif/device"
	"github.com/use-go/onvif/media"
	sdkdevice "github.com/use-go/onvif/sdk/device"
	sdkmedia "github.com/use-go/onvif/sdk/media"
	"github.com/use-go/onvif/xsd/onvif"

	"github.com/gyeongho/go-onvif-tutorial/internal/config"
)

// CameraResult holds the connection result for a single camera.
// In a real VMS, this would be stored in a database and updated
// periodically by a background health-check goroutine.
type CameraResult struct {
	Xaddr        string
	Online       bool
	Error        string
	Manufacturer string
	Model        string
	Firmware     string
	StreamURLs   []string
	ConnectTime  time.Duration
}

func main() {
	fmt.Println("=== Multi-Camera Concurrent Manager ===")
	fmt.Println()

	cameras, err := config.LoadCameras()
	if err != nil {
		log.Fatalf("Failed to load camera config: %v", err)
	}

	fmt.Printf("Configured cameras: %d\n", len(cameras))
	fmt.Println("Connecting to all cameras concurrently...")
	fmt.Println()

	// ─── Concurrent Camera Connection ───────────────────────────────────
	// Use goroutines to connect to all cameras simultaneously.
	// sync.WaitGroup ensures we wait for all connections to complete.
	// A mutex protects the shared results slice from concurrent writes.
	//
	// In production VMS software, you would use:
	//   - A worker pool (e.g., semaphore channel) to limit concurrency
	//   - Context with timeout per camera (e.g., 10 seconds)
	//   - Retry logic with exponential backoff for flaky cameras
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []CameraResult
	)

	for _, cam := range cameras {
		wg.Add(1)
		go func(cam *config.CameraConfig) {
			defer wg.Done()

			result := connectCamera(cam)

			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(cam)
	}

	// Wait for all camera connections to complete.
	// In a real VMS, this would be a background process that runs
	// continuously, not a blocking wait.
	wg.Wait()

	// ─── Results Summary Table ──────────────────────────────────────────
	// Display results in a formatted table using tabwriter, similar
	// to what a VMS dashboard would show.
	fmt.Println("--- Camera Status Summary ---")
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CAMERA IP\tSTATUS\tMANUFACTURER\tMODEL\tFIRMWARE\tCONNECT TIME")
	fmt.Fprintln(w, "---------\t------\t------------\t-----\t--------\t------------")

	onlineCount := 0
	for _, r := range results {
		status := "OFFLINE"
		if r.Online {
			status = "ONLINE"
			onlineCount++
		}

		manufacturer := r.Manufacturer
		if manufacturer == "" {
			manufacturer = "-"
		}
		model := r.Model
		if model == "" {
			model = "-"
		}
		firmware := r.Firmware
		if firmware == "" {
			firmware = "-"
		}

		connectTime := "-"
		if r.Online {
			connectTime = r.ConnectTime.Round(time.Millisecond).String()
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Xaddr, status, manufacturer, model, firmware, connectTime)
	}
	w.Flush()
	fmt.Println()

	// ─── Stream URLs ────────────────────────────────────────────────────
	fmt.Println("--- Stream URLs ---")
	for _, r := range results {
		if !r.Online {
			fmt.Printf("  %s: OFFLINE", r.Xaddr)
			if r.Error != "" {
				fmt.Printf(" (%s)", r.Error)
			}
			fmt.Println()
			continue
		}
		fmt.Printf("  %s:\n", r.Xaddr)
		for i, url := range r.StreamURLs {
			fmt.Printf("    Stream %d: %s\n", i+1, url)
		}
	}
	fmt.Println()

	// ─── Summary ────────────────────────────────────────────────────────
	fmt.Printf("=== Summary: %d/%d cameras online ===\n", onlineCount, len(results))
	fmt.Println()
	fmt.Println("In a real VMS, this data would be:")
	fmt.Println("  - Stored in a database for persistent state")
	fmt.Println("  - Refreshed periodically by background workers")
	fmt.Println("  - Displayed in a web dashboard with live thumbnails")
	fmt.Println("  - Used to trigger alerts when cameras go offline")
}

// connectCamera attempts to connect to a single camera and retrieve
// its device info and stream URLs. Returns a CameraResult with the
// outcome (success or failure details).
func connectCamera(cam *config.CameraConfig) CameraResult {
	result := CameraResult{
		Xaddr: cam.Xaddr(),
	}

	start := time.Now()

	// Connect to the camera
	dev, err := goonvif.NewDevice(goonvif.DeviceParams{
		Xaddr:    cam.Xaddr(),
		Username: cam.Username,
		Password: cam.Password,
	})
	if err != nil {
		result.Error = fmt.Sprintf("connection failed: %v", err)
		return result
	}

	result.ConnectTime = time.Since(start)
	result.Online = true
	ctx := context.Background()

	// Get device information (manufacturer, model, firmware)
	infoResp, err := sdkdevice.Call_GetDeviceInformation(ctx, dev, device.GetDeviceInformation{})
	if err != nil {
		result.Error = fmt.Sprintf("GetDeviceInformation failed: %v", err)
	} else {
		result.Manufacturer = infoResp.Manufacturer
		result.Model = infoResp.Model
		result.Firmware = infoResp.FirmwareVersion
	}

	// Get media profiles and stream URLs
	profilesResp, err := sdkmedia.Call_GetProfiles(ctx, dev, media.GetProfiles{})
	if err != nil {
		if result.Error == "" {
			result.Error = fmt.Sprintf("GetProfiles failed: %v", err)
		}
		return result
	}

	// Get RTSP stream URL for each profile
	for _, profile := range profilesResp.Profiles {
		streamResp, err := sdkmedia.Call_GetStreamUri(ctx, dev, media.GetStreamUri{
			StreamSetup: onvif.StreamSetup{
				Stream: "RTP-Unicast",
				Transport: onvif.Transport{
					Protocol: "RTSP",
				},
			},
			ProfileToken: profile.Token,
		})
		if err != nil {
			continue
		}
		result.StreamURLs = append(result.StreamURLs, string(streamResp.MediaUri.Uri))
	}

	return result
}
