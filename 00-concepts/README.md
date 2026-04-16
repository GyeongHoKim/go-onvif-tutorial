# 00 - ONVIF Concepts

## What This Section Covers

Before writing code, it helps to understand the architecture and protocols that ONVIF is built on. This section covers the theory you need to make sense of the code in subsequent sections.

## What is ONVIF?

**ONVIF** (Open Network Video Interface Forum) is an industry standard for IP-based security products. It defines a common protocol so that cameras, NVRs, and VMS software from different vendors can interoperate without vendor-specific integrations.

In practice, ONVIF means: if your Go application speaks the ONVIF protocol, it can control any ONVIF-conformant camera regardless of manufacturer.

## Protocol Stack

ONVIF communication is built on standard web service technologies:

```
┌─────────────────────────────┐
│     ONVIF Services          │  Device, Media, PTZ, Events, Imaging, Recording
├─────────────────────────────┤
│     SOAP 1.2                │  XML message format with envelope, header, body
├─────────────────────────────┤
│     WS-Security             │  WS-UsernameToken authentication
├─────────────────────────────┤
│     HTTP / HTTPS            │  Transport layer (POST requests)
├─────────────────────────────┤
│     TCP/IP                  │  Network layer
└─────────────────────────────┘
```

## ONVIF Profiles

Profiles define sets of features that a device must implement:

| Profile | Name | Core Features |
|---------|------|---------------|
| S | Streaming | Video streaming, PTZ, audio, multicast |
| G | Recording | On-device recording and playback |
| T | Advanced Streaming | H.265, imaging, motion alarms |
| C | Access Control | Door control, credential management |
| A | Access Control Config | Configure access rules and credentials |
| D | Peripheral Access | Serial/digital I/O, relay outputs |

Most IP cameras implement **Profile S** at minimum. Higher-end cameras and NVRs implement Profile G for recording.

## SOAP Message Structure

Every ONVIF request is a SOAP XML message sent via HTTP POST:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
  <s:Header>
    <!-- WS-UsernameToken goes here -->
  </s:Header>
  <s:Body>
    <tds:GetDeviceInformation/>
  </s:Body>
</s:Envelope>
```

The `use-go/onvif` library constructs and parses these XML messages for you, so you work with Go structs instead of raw XML.

## WS-UsernameToken Authentication

ONVIF uses WS-UsernameToken for authentication. The security header includes:

1. **Username:** Plain text username
2. **Password Digest:** `Base64(SHA1(Nonce + Created + Password))`
3. **Nonce:** A random value to prevent replay attacks
4. **Created:** Timestamp — must be close to the camera's clock

This is why time synchronization between client and camera is critical.

## WS-Discovery

WS-Discovery is a separate protocol used to find devices on the local network:

- Uses **UDP multicast** on `239.255.255.250:3702`
- Client sends a **Probe** message
- Devices respond with **ProbeMatch** containing their service address
- No prior knowledge of device IPs required

## Key Terminology

| Term | Meaning |
|------|---------|
| **XAddr** | Service endpoint URL (e.g., `http://192.168.1.10/onvif/media_service`) |
| **Token** | Identifier for a configuration or profile (e.g., `ProfileToken`, `PresetToken`) |
| **WSDL** | Web Services Description Language — defines the ONVIF service interfaces |
| **Scope** | Device metadata advertised during discovery (name, location, type) |
| **NVR** | Network Video Recorder — stores video from multiple cameras |
| **VMS** | Video Management System — software for viewing and managing camera feeds |

## Next Steps

With these concepts in mind, proceed to [01 - Setup](../01-setup/) to connect to your first camera.
