// 05-ptz: ONVIF PTZ Service — Pan, Tilt, Zoom Control
//
// The PTZ Service (tptz) allows remote control of motorized cameras.
// PTZ stands for Pan (horizontal rotation), Tilt (vertical rotation),
// and Zoom (optical/digital magnification).
//
// PTZ movement types:
//   - ContinuousMove: Camera moves at a given speed until Stop is called.
//     Best for joystick-like UI controls. Speed range is -1.0 to 1.0.
//   - AbsoluteMove: Camera moves to an exact position (e.g., pan=0.5, tilt=-0.3).
//     Best for preset positions and automated patrol routes.
//   - RelativeMove: Camera moves by a delta from current position.
//     Best for "nudge" controls (move 10 degrees left).
//
// PTZ coordinate system:
//   - Pan:  -1.0 (full left) to 1.0 (full right)
//   - Tilt: -1.0 (full down) to 1.0 (full up)
//   - Zoom: 0.0 (wide) to 1.0 (telephoto)
//
// Note: Not all cameras support PTZ. Fixed cameras will return an error
// or empty PTZ node list. PTZ requires a motorized pan/tilt head or a
// camera with digital PTZ (ePTZ) capability.
//
// Run: go run ./05-ptz/
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	goonvif "github.com/use-go/onvif"
	"github.com/use-go/onvif/media"
	"github.com/use-go/onvif/ptz"
	sdkmedia "github.com/use-go/onvif/sdk/media"
	sdkptz "github.com/use-go/onvif/sdk/ptz"
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

	fmt.Println("=== ONVIF PTZ Service ===")
	fmt.Println()

	ctx := context.Background()

	// ─── 1. GetNodes ────────────────────────────────────────────────────
	// PTZ nodes represent physical or virtual pan/tilt/zoom mechanisms.
	// Most cameras have a single PTZ node. Multi-sensor cameras or
	// cameras with both optical and digital PTZ may have multiple nodes.
	fmt.Println("--- 1. PTZ Nodes ---")
	nodesResp, err := sdkptz.Call_GetNodes(ctx, dev, ptz.GetNodes{})
	if err != nil {
		log.Fatalf("GetNodes failed (camera may not support PTZ): %v", err)
	}

	node := nodesResp.PTZNode
	fmt.Printf("  Node Name         : %s\n", node.Name)
	fmt.Printf("  Token             : %s\n", node.Token)
	fmt.Printf("  Home Supported    : %v\n", node.HomeSupported)
	fmt.Printf("  Max Presets       : %d\n", node.MaximumNumberOfPresets)
	fmt.Printf("  Fixed Home        : %v\n", node.FixedHomePosition)
	fmt.Println()

	// ─── 2. Get a profile token for PTZ operations ──────────────────────
	// All PTZ commands require a profile token. We use the first media
	// profile that has a PTZ configuration attached.
	profilesResp, err := sdkmedia.Call_GetProfiles(ctx, dev, media.GetProfiles{})
	if err != nil {
		log.Fatalf("GetProfiles failed: %v", err)
	}

	var profileToken onvif.ReferenceToken
	for _, p := range profilesResp.Profiles {
		if p.PTZConfiguration.Token != "" {
			profileToken = p.Token
			fmt.Printf("Using profile %q (token: %s) for PTZ\n", p.Name, p.Token)
			break
		}
	}
	if profileToken == "" {
		// Fall back to first profile even without explicit PTZ config
		if len(profilesResp.Profiles) > 0 {
			profileToken = profilesResp.Profiles[0].Token
			fmt.Printf("Using first profile (token: %s) for PTZ\n", profileToken)
		} else {
			log.Fatal("No profiles available for PTZ control")
		}
	}
	fmt.Println()

	// ─── 3. GetStatus ───────────────────────────────────────────────────
	// Returns the current PTZ position and movement status.
	// Position values are normalized: Pan [-1, 1], Tilt [-1, 1], Zoom [0, 1]
	printStatus(ctx, dev, profileToken)

	// ─── 4. Interactive PTZ Menu ────────────────────────────────────────
	// This interactive menu demonstrates ContinuousMove for directional
	// control and GotoHomePosition for returning to a saved home position.
	//
	// In a real VMS, these would be triggered by UI controls (joystick,
	// arrow buttons, scroll wheel for zoom).
	fmt.Println("--- 4. Interactive PTZ Control ---")
	fmt.Println()
	fmt.Println("  [1] Move Up        [5] Zoom In")
	fmt.Println("  [2] Move Down      [6] Zoom Out")
	fmt.Println("  [3] Move Left      [7] Go to Home")
	fmt.Println("  [4] Move Right     [8] Stop")
	fmt.Println("  [9] Show Status    [q] Quit")
	fmt.Println()

	// Movement speed: 0.5 is a moderate speed (range: -1.0 to 1.0).
	// In production, this would be adjustable via a slider or joystick input.
	const speed = 0.5

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("PTZ> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		switch input {
		case "1": // Move Up — positive tilt speed
			continuousMove(ctx, dev, profileToken, 0, speed, 0)
			fmt.Println("  Moving UP... (press 8 to stop)")

		case "2": // Move Down — negative tilt speed
			continuousMove(ctx, dev, profileToken, 0, -speed, 0)
			fmt.Println("  Moving DOWN... (press 8 to stop)")

		case "3": // Move Left — negative pan speed
			continuousMove(ctx, dev, profileToken, -speed, 0, 0)
			fmt.Println("  Moving LEFT... (press 8 to stop)")

		case "4": // Move Right — positive pan speed
			continuousMove(ctx, dev, profileToken, speed, 0, 0)
			fmt.Println("  Moving RIGHT... (press 8 to stop)")

		case "5": // Zoom In — positive zoom speed
			continuousMove(ctx, dev, profileToken, 0, 0, speed)
			fmt.Println("  Zooming IN... (press 8 to stop)")

		case "6": // Zoom Out — negative zoom speed
			continuousMove(ctx, dev, profileToken, 0, 0, -speed)
			fmt.Println("  Zooming OUT... (press 8 to stop)")

		case "7": // Go to Home Position
			gotoHome(ctx, dev, profileToken)
			fmt.Println("  Moving to HOME position...")

		case "8": // Stop all movement
			stopMove(ctx, dev, profileToken)
			fmt.Println("  STOPPED")

		case "9": // Print current status
			printStatus(ctx, dev, profileToken)

		case "q", "Q":
			// Stop any ongoing movement before exiting
			stopMove(ctx, dev, profileToken)
			fmt.Println("  Stopped PTZ and exiting.")
			return

		default:
			fmt.Println("  Unknown command. Enter 1-9 or q.")
		}
		fmt.Println()
	}
}

// continuousMove sends a ContinuousMove command with the given pan/tilt/zoom speeds.
// The camera will keep moving until a Stop command is sent.
//
// ContinuousMove is the most common PTZ control method in VMS software because
// it maps naturally to joystick input — the user pushes the stick and the camera
// moves; releasing the stick sends a Stop.
func continuousMove(ctx context.Context, dev *goonvif.Device, token onvif.ReferenceToken, panSpeed, tiltSpeed, zoomSpeed float64) {
	_, err := sdkptz.Call_ContinuousMove(ctx, dev, ptz.ContinuousMove{
		ProfileToken: token,
		Velocity: onvif.PTZSpeed{
			PanTilt: onvif.Vector2D{
				X: panSpeed,
				Y: tiltSpeed,
			},
			Zoom: onvif.Vector1D{
				X: zoomSpeed,
			},
		},
		// Timeout can be set to auto-stop after a duration.
		// Empty means move indefinitely until Stop is called.
	})
	if err != nil {
		log.Printf("  ContinuousMove failed: %v", err)
	}
}

// stopMove sends a Stop command to halt all PTZ movement.
func stopMove(ctx context.Context, dev *goonvif.Device, token onvif.ReferenceToken) {
	_, err := sdkptz.Call_Stop(ctx, dev, ptz.Stop{
		ProfileToken: token,
		PanTilt:      true,
		Zoom:         true,
	})
	if err != nil {
		log.Printf("  Stop failed: %v", err)
	}
}

// gotoHome moves the camera to its configured home position.
// The home position is set by the camera operator and is typically
// the default viewing angle (e.g., looking at a building entrance).
func gotoHome(ctx context.Context, dev *goonvif.Device, token onvif.ReferenceToken) {
	_, err := sdkptz.Call_GotoHomePosition(ctx, dev, ptz.GotoHomePosition{
		ProfileToken: token,
	})
	if err != nil {
		log.Printf("  GotoHomePosition failed: %v", err)
	}
}

// printStatus queries and prints the current PTZ position.
func printStatus(ctx context.Context, dev *goonvif.Device, token onvif.ReferenceToken) {
	fmt.Println("--- PTZ Status ---")
	statusResp, err := sdkptz.Call_GetStatus(ctx, dev, ptz.GetStatus{
		ProfileToken: token,
	})
	if err != nil {
		log.Printf("  GetStatus failed: %v", err)
		return
	}

	pos := statusResp.PTZStatus.Position
	fmt.Printf("  Pan  : %.4f\n", pos.PanTilt.X)
	fmt.Printf("  Tilt : %.4f\n", pos.PanTilt.Y)
	fmt.Printf("  Zoom : %.4f\n", pos.Zoom.X)

	ms := statusResp.PTZStatus.MoveStatus
	fmt.Printf("  Pan/Tilt Status: %s\n", ms.PanTilt)
	fmt.Printf("  Zoom Status    : %s\n", ms.Zoom)
	fmt.Println()

	// ─── AbsoluteMove example (commented out) ───────────────────────────
	// AbsoluteMove moves the camera to an exact position. This is used for
	// preset positions in a VMS — e.g., "Entrance View" = pan:0.3, tilt:-0.1, zoom:0.5
	//
	// Example:
	//   sdkptz.Call_AbsoluteMove(ctx, dev, ptz.AbsoluteMove{
	//       ProfileToken: token,
	//       Position: onvif.PTZVector{
	//           PanTilt: onvif.Vector2D{X: 0.3, Y: -0.1},
	//           Zoom:    onvif.Vector1D{X: 0.5},
	//       },
	//       Speed: onvif.PTZSpeed{
	//           PanTilt: onvif.Vector2D{X: 1.0, Y: 1.0},
	//           Zoom:    onvif.Vector1D{X: 1.0},
	//       },
	//   })
}
