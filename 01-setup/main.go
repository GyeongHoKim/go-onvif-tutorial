// 01-setup: Connecting to an ONVIF Camera
//
// This example demonstrates the very first step in any ONVIF integration:
// establishing a connection to an ONVIF-compliant IP camera or NVR.
//
// What happens when you call onvif.NewDevice():
//   1. The library constructs the ONVIF Device Service URL from the Xaddr
//      (e.g., http://192.168.1.100:80/onvif/device_service).
//   2. It sends a GetCapabilities SOAP request to discover all available
//      service endpoints (Media, PTZ, Events, Imaging, etc.).
//   3. It stores the discovered endpoints internally so future CallMethod()
//      calls are automatically routed to the correct service URL.
//   4. If Username and Password are provided, all subsequent SOAP requests
//      will include WS-UsernameToken authentication headers with a nonce,
//      timestamp, and password digest (SHA-1).
//
// Prerequisites:
//   - Copy .env.example to .env.local and fill in your camera's IP, port,
//     username, and password.
//   - The camera must be reachable on the network.
//   - Run: go run ./01-setup/
package main

import (
	"context"
	"fmt"
	"log"

	goonvif "github.com/use-go/onvif"
	"github.com/use-go/onvif/device"
	sdk "github.com/use-go/onvif/sdk/device"

	"github.com/gyeongho/go-onvif-tutorial/internal/config"
)

func main() {
	// Step 1: Load camera credentials from .env.local
	// We never hard-code credentials — they come from environment files.
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Println("=== ONVIF Camera Setup ===")
	fmt.Printf("Connecting to camera at %s ...\n\n", cfg.Xaddr())

	// Step 2: Create an ONVIF device connection
	//
	// NewDevice() performs the initial handshake with the camera:
	// - Sends GetCapabilities(Category="All") to discover service endpoints
	// - Stores endpoint URLs for Media, PTZ, Events, Imaging, etc.
	// - If the camera is unreachable or not ONVIF-compliant, this returns an error.
	//
	// The Xaddr format is "host:port" — the library prepends http:// and
	// appends /onvif/device_service automatically.
	dev, err := goonvif.NewDevice(goonvif.DeviceParams{
		Xaddr:    cfg.Xaddr(),
		Username: cfg.Username,
		Password: cfg.Password,
	})
	if err != nil {
		log.Fatalf("Failed to connect to camera: %v\n"+
			"Make sure the camera is online and credentials are correct in .env.local", err)
	}

	fmt.Println("Successfully connected to the ONVIF device!")
	fmt.Println()

	// Step 3: List the discovered service endpoints
	// These are the ONVIF services the camera supports. Each service has
	// its own URL endpoint for SOAP requests.
	fmt.Println("--- Discovered Service Endpoints ---")
	services := dev.GetServices()
	for name, url := range services {
		fmt.Printf("  %-12s -> %s\n", name, url)
	}
	fmt.Println()

	// Step 4: Get device information to verify the connection works
	// GetDeviceInformation returns the camera's manufacturer, model,
	// firmware version, serial number, and hardware ID.
	getInfo := device.GetDeviceInformation{}
	resp, err := sdk.Call_GetDeviceInformation(context.Background(), dev, getInfo)
	if err != nil {
		log.Fatalf("Failed to get device information: %v", err)
	}

	fmt.Println("--- Device Information ---")
	fmt.Printf("  Manufacturer   : %s\n", resp.Manufacturer)
	fmt.Printf("  Model          : %s\n", resp.Model)
	fmt.Printf("  Firmware       : %s\n", resp.FirmwareVersion)
	fmt.Printf("  Serial Number  : %s\n", resp.SerialNumber)
	fmt.Printf("  Hardware ID    : %s\n", resp.HardwareId)
	fmt.Println()

	fmt.Println("Setup complete! Your camera is ready for ONVIF operations.")
	fmt.Println("Try the next examples: 02-device-management, 03-discovery, 04-media, etc.")
}
