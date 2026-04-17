// 03-discovery: WS-Discovery — Finding ONVIF Cameras on the Network
//
// WS-Discovery is a protocol that allows clients to find ONVIF devices
// on the local network without knowing their IP addresses in advance.
//
// How WS-Discovery works:
//  1. The client sends a SOAP "Probe" message via UDP multicast to:
//     Address : 239.255.255.250 (standard multicast group)
//     Port    : 3702 (IANA-assigned for WS-Discovery)
//  2. Every ONVIF device on the local subnet that receives the Probe
//     responds with a unicast "ProbeMatch" message containing:
//     - XAddrs: the device's ONVIF service URL(s)
//     - Types:  what kind of device it is (e.g., NetworkVideoTransmitter)
//     - Scopes: additional metadata (location, name, hardware info)
//  3. The client collects responses for about 1 second (configurable)
//     and builds a list of discovered devices.
//
// Why multicast?
//
//	Multicast allows a single probe to reach ALL devices on the subnet
//	simultaneously, without needing to scan IP ranges. This is much
//	faster and more efficient than port-scanning 192.168.1.1–254.
//
// Network requirements:
//   - The client and cameras must be on the same subnet (or multicast
//     routing must be configured across subnets).
//   - Firewalls must allow UDP traffic on port 3702.
//   - IGMP snooping on switches should be configured properly.
//
// Run: go run ./03-discovery/
package main

import (
	"fmt"
	"log"
	"net"

	goonvif "github.com/use-go/onvif"
)

func main() {
	fmt.Println("=== ONVIF WS-Discovery ===")
	fmt.Println()
	fmt.Println("Scanning for ONVIF devices on the local network...")
	fmt.Println("This sends a WS-Discovery Probe via UDP multicast to 239.255.255.250:3702")
	fmt.Println()

	// List available network interfaces so the user knows which one to use.
	// WS-Discovery requires specifying the network interface because multicast
	// is interface-specific — a machine with both Ethernet and Wi-Fi would
	// need to probe on each interface separately.
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Fatalf("Failed to list network interfaces: %v", err)
	}

	fmt.Println("--- Available Network Interfaces ---")
	var candidates []net.Interface
	for _, iface := range ifaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil || len(addrs) == 0 {
			continue
		}

		addrStrs := make([]string, 0)
		for _, addr := range addrs {
			addrStrs = append(addrStrs, addr.String())
		}

		fmt.Printf("  [%d] %s (MAC: %s)\n", len(candidates), iface.Name, iface.HardwareAddr)
		for _, a := range addrStrs {
			fmt.Printf("      IP: %s\n", a)
		}
		candidates = append(candidates, iface)
	}
	fmt.Println()

	if len(candidates) == 0 {
		log.Fatal("No suitable network interfaces found")
	}

	// Probe on each candidate interface
	totalDevices := 0
	for _, iface := range candidates {
		fmt.Printf("--- Probing on interface: %s ---\n", iface.Name)

		// GetAvailableDevicesAtSpecificEthernetInterface sends a WS-Discovery
		// Probe for "NetworkVideoTransmitter" (NVT) type devices.
		// It uses the sendUDPMulticast function internally which:
		//   1. Opens a UDP socket on 0.0.0.0:0 (any available port)
		//   2. Joins the multicast group 239.255.255.250 on the specified interface
		//   3. Sends the Probe XML message to 239.255.255.250:3702
		//   4. Waits ~1 second for ProbeMatch responses
		//   5. Parses XAddrs from each ProbeMatch to create Device objects
		devices, err := goonvif.GetAvailableDevicesAtSpecificEthernetInterface(iface.Name)
		if err != nil {
			fmt.Printf("  Error probing on %s: %v\n", iface.Name, err)
			fmt.Println()
			continue
		}

		if len(devices) == 0 {
			fmt.Println("  No devices found on this interface")
			fmt.Println()
			continue
		}

		fmt.Printf("  Found %d device(s):\n", len(devices))
		for i, dev := range devices {
			totalDevices++
			info := dev.GetDeviceInfo()
			services := dev.GetServices()

			fmt.Printf("\n  Device #%d:\n", i+1)
			if info.Manufacturer != "" {
				fmt.Printf("    Manufacturer : %s\n", info.Manufacturer)
				fmt.Printf("    Model        : %s\n", info.Model)
				fmt.Printf("    Firmware     : %s\n", info.FirmwareVersion)
			}

			fmt.Println("    Service Endpoints:")
			for svc, url := range services {
				fmt.Printf("      %-10s : %s\n", svc, url)
			}
		}
		fmt.Println()
	}

	fmt.Println("=== Discovery Summary ===")
	fmt.Printf("Total devices found: %d\n", totalDevices)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  - Use the discovered XAddrs to connect with onvif.NewDevice()")
	fmt.Println("  - See 01-setup/ for connection examples")
	fmt.Println("  - See 04-media/ for getting stream URLs from discovered devices")
}
