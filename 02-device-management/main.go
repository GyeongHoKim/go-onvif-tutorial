// 02-device-management: ONVIF Device Management Service
//
// The Device Management Service (tds) is the foundational ONVIF service.
// Every ONVIF-compliant device MUST implement it. It provides:
//
//   - Device identification (manufacturer, model, firmware, serial number)
//   - Capability discovery (which ONVIF services the device supports)
//   - Network configuration (interfaces, DNS, NTP, gateway)
//   - System operations (date/time, reboot, logs, factory reset)
//   - User management (create, delete, modify users)
//   - Security (certificates, access policies)
//
// The Device Management Service endpoint is always at:
//
//	http://<camera-ip>:<port>/onvif/device_service
//
// This example demonstrates the most commonly used operations.
//
// Run: go run ./02-device-management/
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"

	goonvif "github.com/use-go/onvif"
	"github.com/use-go/onvif/device"
	sdk "github.com/use-go/onvif/sdk/device"

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

	fmt.Println("=== ONVIF Device Management Service ===")
	fmt.Println()

	ctx := context.Background()

	// ─── 1. GetDeviceInformation ────────────────────────────────────────
	// Returns the camera's manufacturer, model, firmware version, serial
	// number, and hardware ID. This is typically the first call a VMS
	// makes after discovering a device to identify what it's talking to.
	fmt.Println("--- 1. Device Information ---")
	infoResp, err := sdk.Call_GetDeviceInformation(ctx, dev, device.GetDeviceInformation{})
	if err != nil {
		log.Printf("  GetDeviceInformation failed: %v", err)
	} else {
		fmt.Printf("  Manufacturer    : %s\n", infoResp.Manufacturer)
		fmt.Printf("  Model           : %s\n", infoResp.Model)
		fmt.Printf("  Firmware Version: %s\n", infoResp.FirmwareVersion)
		fmt.Printf("  Serial Number   : %s\n", infoResp.SerialNumber)
		fmt.Printf("  Hardware ID     : %s\n", infoResp.HardwareId)
	}
	fmt.Println()

	// ─── 2. GetCapabilities ─────────────────────────────────────────────
	// Returns all the ONVIF service categories this device supports and
	// their endpoint URLs. Categories include: Analytics, Device, Events,
	// Imaging, Media, PTZ, and Extension services.
	//
	// This is how a VMS discovers what features a camera has:
	//   - Does it support PTZ? Check if PTZ capability is present.
	//   - Does it support events? Check the Events capability.
	//   - Does it support recording? Check for Extension services.
	fmt.Println("--- 2. Device Capabilities ---")
	capResp, err := sdk.Call_GetCapabilities(ctx, dev, device.GetCapabilities{Category: "All"})
	if err != nil {
		log.Printf("  GetCapabilities failed: %v", err)
	} else {
		caps := capResp.Capabilities

		if string(caps.Device.XAddr) != "" {
			fmt.Printf("  Device Service   : %s\n", caps.Device.XAddr)
		}
		if string(caps.Media.XAddr) != "" {
			fmt.Printf("  Media Service    : %s\n", caps.Media.XAddr)
		}
		if string(caps.Events.XAddr) != "" {
			fmt.Printf("  Events Service   : %s\n", caps.Events.XAddr)
		}
		if string(caps.PTZ.XAddr) != "" {
			fmt.Printf("  PTZ Service      : %s\n", caps.PTZ.XAddr)
		}
		if string(caps.Imaging.XAddr) != "" {
			fmt.Printf("  Imaging Service  : %s\n", caps.Imaging.XAddr)
		}
		if string(caps.Analytics.XAddr) != "" {
			fmt.Printf("  Analytics Service: %s\n", caps.Analytics.XAddr)
		}
	}
	fmt.Println()

	// ─── 3. GetSystemDateAndTime ────────────────────────────────────────
	// Returns the camera's current date, time, and timezone settings.
	//
	// This is CRITICAL for ONVIF authentication! The WS-UsernameToken
	// security scheme includes a timestamp in each SOAP request. If the
	// camera's clock and the client's clock differ by more than a few
	// seconds, authentication will fail with "Sender not Authorized".
	//
	// A VMS typically checks the camera time on connection and warns
	// the user if there is significant clock drift.
	fmt.Println("--- 3. System Date and Time ---")
	dtResp, err := sdk.Call_GetSystemDateAndTime(ctx, dev, device.GetSystemDateAndTime{})
	if err != nil {
		log.Printf("  GetSystemDateAndTime failed: %v", err)
	} else {
		sdt := dtResp.SystemDateAndTime
		fmt.Printf("  Date/Time Type   : %s\n", sdt.DateTimeType)
		fmt.Printf("  Daylight Savings : %v\n", sdt.DaylightSavings)
		if sdt.TimeZone.TZ != "" {
			fmt.Printf("  Timezone         : %s\n", sdt.TimeZone.TZ)
		}
		// UTCDateTime is an xsd.DateTime string (e.g., "2024-01-15T10:30:00Z")
		// Some cameras return it as a structured XML element, others as a string.
		// We print it as-is since the xsd.DateTime type is a simple string alias.
		fmt.Printf("  UTC DateTime     : %s\n", sdt.UTCDateTime)
		fmt.Printf("  Local DateTime   : %s\n", sdt.LocalDateTime)
	}
	fmt.Println()

	// ─── 4. GetNetworkInterfaces ────────────────────────────────────────
	// Returns the camera's network interface configuration including
	// MAC address, IPv4/IPv6 settings, link state, and MTU.
	// Useful for network diagnostics and multi-NIC camera setups.
	fmt.Println("--- 4. Network Interfaces ---")
	niResp, err := sdk.Call_GetNetworkInterfaces(ctx, dev, device.GetNetworkInterfaces{})
	if err != nil {
		log.Printf("  GetNetworkInterfaces failed: %v", err)
	} else {
		ni := niResp.NetworkInterfaces
		fmt.Printf("  Interface Token  : %s\n", ni.Token)
		fmt.Printf("  Enabled          : %v\n", ni.Enabled)
		if ni.Info.Name != "" {
			fmt.Printf("  Name             : %s\n", ni.Info.Name)
		}
		if ni.Info.HwAddress != "" {
			fmt.Printf("  MAC Address      : %s\n", ni.Info.HwAddress)
		}
	}
	fmt.Println()

	// ─── 5. GetSystemLog ────────────────────────────────────────────────
	// Retrieves the system log from the device. The log type can be
	// "System" or "Access". Not all cameras support this operation.
	//
	// System logs are useful for debugging camera issues, checking for
	// unauthorized access attempts, or reviewing firmware update history.
	fmt.Println("--- 5. System Log (raw SOAP, may not be supported) ---")
	getLog := device.GetSystemLog{LogType: "System"}
	resp, err := dev.CallMethod(getLog)
	if err != nil {
		log.Printf("  GetSystemLog failed: %v", err)
	} else {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusOK {
			// Truncate long log output for readability
			logStr := string(body)
			if len(logStr) > 500 {
				logStr = logStr[:500] + "\n  ... (truncated)"
			}
			fmt.Printf("  %s\n", logStr)
		} else {
			fmt.Printf("  Device returned HTTP %d — GetSystemLog may not be supported\n", resp.StatusCode)
		}
	}
	fmt.Println()

	fmt.Println("Device management exploration complete!")
}
