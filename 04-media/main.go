// 04-media: ONVIF Media Service — Profiles, Stream URLs, and Snapshots
//
// The Media Service (trt) is the most important ONVIF service for video
// management systems (VMS). It provides:
//
//   - Media Profiles: Named configurations that combine a video source,
//     video encoder, audio source/encoder, PTZ config, and analytics.
//     A profile is essentially a "recipe" for how to produce a video stream.
//
//   - Stream URIs: RTSP URLs that a VMS or media player uses to receive
//     live video. The camera generates these based on the profile config.
//
//   - Snapshot URIs: HTTP URLs that return a single JPEG frame from the
//     camera. Useful for thumbnails, previews, and motion-triggered captures.
//
// Typical VMS workflow:
//  1. GetProfiles() to list available media profiles
//  2. For each profile, GetStreamUri() to get the RTSP URL
//  3. Connect to the RTSP URL with a media player (VLC, ffmpeg, GStreamer)
//  4. Optionally GetSnapshotUri() for thumbnail generation
//
// Stream transport options:
//   - UDP: Lower latency, may lose packets on congested networks
//   - TCP: Reliable delivery, slightly higher latency
//   - RTSP over HTTP: Works through HTTP proxies and firewalls
//
// Run: go run ./04-media/
package main

import (
	"context"
	"fmt"
	"log"

	goonvif "github.com/use-go/onvif"
	"github.com/use-go/onvif/media"
	sdkmedia "github.com/use-go/onvif/sdk/media"
	"github.com/use-go/onvif/xsd/onvif"

	"github.com/gyeongho/go-onvif-tutorial/internal/config"
)

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

	fmt.Println("=== ONVIF Media Service ===")
	fmt.Println()

	ctx := context.Background()

	// ─── 1. GetProfiles ─────────────────────────────────────────────────
	// Returns all media profiles configured on the camera.
	//
	// Most cameras have at least two profiles:
	//   - "MainStream" or "Profile1": High resolution, high bitrate (for recording)
	//   - "SubStream" or "Profile2": Low resolution, low bitrate (for live preview)
	//
	// Each profile has a unique Token that is used as a reference in all
	// subsequent media operations (GetStreamUri, GetSnapshotUri, PTZ, etc.)
	fmt.Println("--- 1. Media Profiles ---")
	profilesResp, err := sdkmedia.Call_GetProfiles(ctx, dev, media.GetProfiles{})
	if err != nil {
		log.Fatalf("GetProfiles failed: %v", err)
	}

	profiles := profilesResp.Profiles
	if len(profiles) == 0 {
		log.Fatal("No media profiles found on this camera")
	}

	for i, profile := range profiles {
		fmt.Printf("\n  Profile #%d:\n", i+1)
		fmt.Printf("    Name     : %s\n", profile.Name)
		fmt.Printf("    Token    : %s\n", profile.Token)
		fmt.Printf("    Fixed    : %v\n", profile.Fixed)

		// Video Encoder Configuration shows the codec, resolution, and bitrate
		vec := profile.VideoEncoderConfiguration
		if vec.Name != "" {
			fmt.Printf("    Encoder  : %s\n", vec.Name)
			fmt.Printf("    Codec    : %s\n", vec.Encoding)
			fmt.Printf("    Resolution: %dx%d\n", vec.Resolution.Width, vec.Resolution.Height)
			fmt.Printf("    Quality  : %.0f\n", vec.Quality)
			fmt.Printf("    FPS Limit: %d\n", vec.RateControl.FrameRateLimit)
			fmt.Printf("    Bitrate  : %d kbps\n", vec.RateControl.BitrateLimit)
		}
	}
	fmt.Println()

	// ─── 2. GetStreamUri ────────────────────────────────────────────────
	// For each profile, request the RTSP stream URL.
	//
	// StreamSetup specifies:
	//   - Stream: "RTP-Unicast" (one-to-one) or "RTP-Multicast" (one-to-many)
	//   - Transport Protocol: "UDP", "TCP", "RTSP", or "HTTP"
	//
	// The returned URI is what you feed into VLC, ffmpeg, or any RTSP client:
	//   vlc rtsp://192.168.1.100:554/stream1
	//   ffplay rtsp://192.168.1.100:554/stream1
	fmt.Println("--- 2. Stream URIs (RTSP) ---")
	for i, profile := range profiles {
		fmt.Printf("\n  Profile: %s (Token: %s)\n", profile.Name, profile.Token)

		// Request RTSP over UDP (lowest latency, standard for LAN)
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
			log.Printf("    GetStreamUri (RTSP) failed for profile %d: %v", i+1, err)
			continue
		}
		fmt.Printf("    RTSP URL : %s\n", streamResp.MediaUri.Uri)

		// Also try TCP transport (more reliable on congested networks)
		streamRespTCP, err := sdkmedia.Call_GetStreamUri(ctx, dev, media.GetStreamUri{
			StreamSetup: onvif.StreamSetup{
				Stream: "RTP-Unicast",
				Transport: onvif.Transport{
					Protocol: "TCP",
				},
			},
			ProfileToken: profile.Token,
		})
		if err == nil {
			fmt.Printf("    TCP URL  : %s\n", streamRespTCP.MediaUri.Uri)
		}
	}
	fmt.Println()

	// ─── 3. GetSnapshotUri ──────────────────────────────────────────────
	// Returns an HTTP URL that serves a single JPEG frame from the camera.
	//
	// This is useful for:
	//   - Generating thumbnails in a camera list UI
	//   - Capturing snapshots on motion detection events
	//   - Low-bandwidth preview when RTSP streaming is not needed
	//
	// Note: The snapshot URL usually requires HTTP Basic or Digest auth
	// with the same credentials used for ONVIF.
	fmt.Println("--- 3. Snapshot URIs ---")
	for i, profile := range profiles {
		snapResp, err := sdkmedia.Call_GetSnapshotUri(ctx, dev, media.GetSnapshotUri{
			ProfileToken: profile.Token,
		})
		if err != nil {
			log.Printf("  GetSnapshotUri failed for profile %d: %v", i+1, err)
			continue
		}
		fmt.Printf("  Profile %q: %s\n", profile.Name, snapResp.MediaUri.Uri)
	}
	fmt.Println()

	// ─── Summary ────────────────────────────────────────────────────────
	fmt.Println("=== Summary ===")
	fmt.Printf("Found %d media profile(s)\n", len(profiles))
	fmt.Println()
	fmt.Println("To view a stream, use the RTSP URL above with:")
	fmt.Println("  vlc <RTSP_URL>")
	fmt.Println("  ffplay <RTSP_URL>")
	fmt.Println("  mpv <RTSP_URL>")
	fmt.Println()
	fmt.Println("To grab a snapshot:")
	fmt.Println("  curl -u admin:password <SNAPSHOT_URL> -o snapshot.jpg")
}
