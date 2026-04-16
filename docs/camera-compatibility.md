# Camera Compatibility Notes

## ONVIF Conformance

Not all cameras that claim "ONVIF support" implement the specification the same way. The ONVIF organization maintains a [conformant products list](https://www.onvif.org/conformant-products/) where you can verify whether your device has passed official conformance testing.

### How to Check If a Camera Is ONVIF Conformant

1. Visit the [ONVIF Conformant Products](https://www.onvif.org/conformant-products/) page.
2. Search by vendor name or model number.
3. Check which **profiles** the device supports (S, G, T, C, etc.):
   - **Profile S** — Streaming (most common, required for basic video)
   - **Profile G** — Recording and storage
   - **Profile T** — Advanced streaming (H.265, imaging)
   - **Profile C** — Access control
   - **Profile A** — Access control configuration
   - **Profile D** — Peripheral device access
4. The conformance level indicates which services and features are guaranteed to work.

## Common Quirks by Vendor

### Axis

- Generally excellent ONVIF compliance — often the reference implementation.
- Uses port 80 by default for ONVIF services.
- VAPIX (proprietary API) is often more feature-rich than their ONVIF implementation for advanced features.
- Some older models require ONVIF to be explicitly enabled under **System > ONVIF** in the web UI.

### Hikvision

- ONVIF must be enabled manually: **Configuration > Network > Advanced Settings > Integration Protocol**.
- You may need to create a separate ONVIF user account in the same settings page.
- Common ONVIF ports: 80, 8080, or 8899 depending on model and firmware.
- Some models return non-standard XML namespaces, which can cause parsing issues.
- WS-Discovery may not work reliably on all firmware versions.

### Dahua

- ONVIF is typically found under **Settings > Network > ONVIF**.
- Like Hikvision, may require a dedicated ONVIF user account.
- Default ONVIF port is often 80.
- Some models have incomplete Profile G (recording) support.
- Event service implementation can differ from the specification — test `PullMessages` carefully.

### Hanwha (Samsung Wisenet)

- Generally good ONVIF conformance across their product line.
- ONVIF settings are under **Setup > Network > ONVIF** in the web UI.
- Supports Profile S and G on most models.
- PTZ behavior is well-implemented on PTZ models.

### Bosch

- Solid ONVIF conformance, particularly on newer models.
- May use non-standard endpoint paths — use `GetCapabilities` to discover service URLs rather than hardcoding them.
- Some models require HTTPS for ONVIF — be prepared to handle TLS.
- Firmware updates frequently improve ONVIF compatibility.

## General Compatibility Tips

1. **Always use `GetCapabilities` or `GetServices` to discover service endpoints.** Never hardcode paths like `/onvif/media_service` — they vary by vendor.

2. **Test `GetDeviceInformation` first.** This is the simplest authenticated call and confirms basic connectivity and authentication are working.

3. **Check the ONVIF version.** Use `GetServices` with `IncludeCapability=true` to see which specification version the device implements. Newer versions support more features.

4. **Handle optional fields gracefully.** Many response fields are optional in the ONVIF specification. Your code should not panic if a field is nil or empty.

5. **Firmware matters.** If a feature doesn't work as expected, check if a firmware update is available. Vendors frequently fix ONVIF bugs in firmware releases.

6. **Have a test camera.** Avoid developing against production cameras. Use a dedicated test device or the ONVIF virtual camera simulator for initial development.
