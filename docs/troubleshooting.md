# Troubleshooting Guide

Common issues you may encounter when working with ONVIF cameras and how to resolve them.

## Authentication Failures

### Wrong Credentials

**Symptom:** HTTP 401 Unauthorized or SOAP Fault with `ter:NotAuthorized`.

**Solutions:**
- Double-check the username and password in your `.env.local` file.
- Some cameras have separate ONVIF user accounts — the web UI credentials may not work for ONVIF. Check the camera's ONVIF user settings page.
- Try the default credentials for your camera vendor (often `admin` / `admin` or `admin` / `12345`).

### Time Sync Issues (WS-UsernameToken)

**Symptom:** Authentication fails even with correct credentials. Error may mention `ter:NotAuthorized` or token expiration.

**Explanation:** ONVIF uses WS-UsernameToken for authentication, which includes a timestamp. If the clock on your machine and the camera differ by more than a few seconds, the token will be rejected.

**Solutions:**
- Sync your computer's clock with an NTP server.
- Use `GetSystemDateAndTime` (which does not require authentication) to read the camera's clock, then compute the time offset and apply it when creating the security header.
- Configure the camera to use the same NTP server as your machine.

### Digest vs. UsernameToken

**Symptom:** One authentication method works but the other doesn't.

**Explanation:** Some cameras only support WS-UsernameToken, while others prefer HTTP Digest. The `use-go/onvif` library handles this automatically in most cases, but older firmware may behave unexpectedly.

## Connection Refused

### Port Configuration

**Symptom:** `connection refused` or `no route to host`.

**Common Ports:**
| Port | Usage |
|------|-------|
| 80   | Default HTTP / ONVIF |
| 8080 | Alternate HTTP for some vendors |
| 8899 | Some Hikvision models |
| 443  | HTTPS (if enabled) |

**Solutions:**
- Verify the camera is reachable: `ping <camera_ip>`
- Try different ports: `curl -v http://<camera_ip>:80/onvif/device_service`
- Check that no firewall is blocking the connection.
- Some cameras disable ONVIF by default — enable it in the camera's web interface.

### HTTPS / TLS Issues

**Symptom:** `tls: handshake failure` or certificate errors.

**Solutions:**
- For development, you can skip TLS verification (not recommended for production).
- Import the camera's self-signed certificate into your system trust store.
- Some cameras use outdated TLS versions — check camera firmware updates.

## WS-Discovery Not Finding Cameras

### Multicast Issues

**Symptom:** `DiscoverDevices` returns an empty list even though cameras are on the network.

**Explanation:** WS-Discovery uses UDP multicast (239.255.255.250:3702). Many network configurations block multicast traffic.

**Solutions:**
- Ensure your machine and cameras are on the same subnet/VLAN.
- Check that your firewall allows UDP traffic on port 3702.
- On macOS, check that the multicast route exists: `netstat -rn | grep 239.255`
- On Linux, you may need: `sudo ip route add 239.255.255.250/32 dev eth0`
- Try increasing the discovery timeout — some cameras respond slowly.

### VLANs and Network Segmentation

**Symptom:** Cameras are visible from one machine but not another.

**Solutions:**
- Confirm both machines are on the same VLAN as the cameras.
- If cameras are on a separate VLAN, configure your router/switch to relay multicast (IGMP snooping / multicast routing).
- As a fallback, connect to cameras directly by IP address instead of using discovery.

### Wireless Issues

Multicast traffic is frequently dropped on Wi-Fi networks. If possible, connect your development machine to the network via Ethernet.

## PTZ Commands Not Working

### Camera Doesn't Support PTZ

**Symptom:** SOAP Fault with `ter:ActionNotSupported` or empty PTZ node list.

**Solutions:**
- Verify your camera has PTZ capabilities (motorized or digital PTZ).
- Check the camera's `GetCapabilities` response for a PTZ service URL.
- Fixed cameras may support digital PTZ (ePTZ) — check `GetNodes` for a digital PTZ node.

### Wrong Profile Token

**Symptom:** PTZ commands return an error about invalid profile.

**Solutions:**
- Use `GetProfiles` from the Media service to get valid profile tokens.
- Ensure the profile you're using has a PTZ configuration attached.
- Some cameras have multiple profiles — use the one with PTZ enabled.

### Movement Range Limits

**Symptom:** Camera doesn't move or moves unexpectedly.

**Solutions:**
- Check `GetConfigurationOptions` for valid speed and position ranges.
- Pan/Tilt values are typically normalized to -1.0 to 1.0.
- Zoom values are typically 0.0 to 1.0.
- Some cameras have mechanical limits — check physical obstructions.

## RTSP Stream Issues

### Stream URI Not Working

**Symptom:** VLC or ffmpeg cannot connect to the RTSP stream URI returned by `GetStreamUri`.

**Solutions:**
- Verify the RTSP port is accessible (default 554): `nc -zv <camera_ip> 554`
- Some cameras require authentication for RTSP — use the format: `rtsp://user:pass@ip:554/path`
- Try both TCP and UDP transport: `ffmpeg -rtsp_transport tcp -i <uri> -frames 1 test.jpg`
- Check if the camera limits concurrent RTSP connections.

### Poor Video Quality or Lag

**Solutions:**
- Check the encoder configuration — high resolution at low bitrate causes artifacts.
- Use `GetVideoEncoderConfiguration` to inspect and adjust settings.
- For low-latency viewing, prefer H.264 Baseline profile over Main/High.
- Ensure your network bandwidth can sustain the configured bitrate.

## General Debugging Tips

1. **Enable HTTP logging:** Inspect the raw SOAP XML being sent and received. The `use-go/onvif` library supports this via Go's `http.Transport`.

2. **Use ONVIF Device Manager:** The free Windows tool [ONVIF Device Manager](https://sourceforge.net/projects/onvifdm/) is useful for verifying what your camera supports.

3. **Check firmware version:** Many ONVIF issues are fixed by firmware updates. Check your camera vendor's support page.

4. **Read the SOAP faults:** ONVIF error responses include structured fault codes (e.g., `ter:InvalidArgVal`, `ter:ActionNotSupported`) that indicate the exact problem.
