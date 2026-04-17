// 07-imaging: ONVIF Imaging Service — Camera Image Settings
//
// The Imaging Service (timg) controls the camera's image processing
// pipeline — the settings that determine how the raw sensor data is
// converted into a visible video image.
//
// Available settings (varies by camera):
//   - Brightness: Overall image brightness (0-100 or 0-255 range)
//   - Contrast: Difference between dark and light areas
//   - Sharpness: Edge enhancement level
//   - Color Saturation: Intensity of colors (0 = grayscale)
//   - Wide Dynamic Range (WDR): Handles scenes with both bright and dark areas
//   - IR Cut Filter: Switches between day mode (color) and night mode (B&W IR)
//   - Backlight Compensation: Adjusts exposure for backlit subjects
//   - White Balance: Color temperature correction (auto or manual)
//   - Exposure: Shutter speed, gain, iris control
//   - Focus: Auto-focus or manual focus control
//
// These settings are per-VideoSource (physical sensor), not per-profile.
// Changing brightness on one profile affects all profiles that use the
// same video source.
//
// Run: go run ./07-imaging/
package main

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	goonvif "github.com/use-go/onvif"
	imaging "github.com/use-go/onvif/Imaging"
	"github.com/use-go/onvif/media"
	sdkmedia "github.com/use-go/onvif/sdk/media"
	"github.com/use-go/onvif/xsd/onvif"

	"github.com/gyeongho/go-onvif-tutorial/internal/config"

	"context"
)

// SOAP response parsing types for Imaging service.
// The use-go/onvif library does not have SDK wrappers for Imaging,
// so we parse XML responses manually.
type soapEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    soapBody `xml:"Body"`
}

type soapBody struct {
	GetImagingSettingsResponse getImagingSettingsResp `xml:"GetImagingSettingsResponse"`
}

type getImagingSettingsResp struct {
	ImagingSettings imagingSettings `xml:"ImagingSettings"`
}

type imagingSettings struct {
	Brightness            float64               `xml:"Brightness"`
	ColorSaturation       float64               `xml:"ColorSaturation"`
	Contrast              float64               `xml:"Contrast"`
	Sharpness             float64               `xml:"Sharpness"`
	BacklightCompensation backlightCompensation `xml:"BacklightCompensation"`
	WideDynamicRange      wideDynamicRange      `xml:"WideDynamicRange"`
	IrCutFilter           string                `xml:"IrCutFilter"`
	WhiteBalance          whiteBalance          `xml:"WhiteBalance"`
}

type backlightCompensation struct {
	Mode  string  `xml:"Mode"`
	Level float64 `xml:"Level"`
}

type wideDynamicRange struct {
	Mode  string  `xml:"Mode"`
	Level float64 `xml:"Level"`
}

type whiteBalance struct {
	Mode string `xml:"Mode"`
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

	fmt.Println("=== ONVIF Imaging Service ===")
	fmt.Println()

	ctx := context.Background()

	// ─── 1. GetVideoSources ─────────────────────────────────────────────
	// A VideoSource represents a physical image sensor on the camera.
	// Most cameras have one, but multi-sensor cameras (panoramic, dual-lens)
	// may have several. Imaging settings are per-VideoSource.
	fmt.Println("--- 1. Video Sources ---")
	vsResp, err := sdkmedia.Call_GetVideoSources(ctx, dev, media.GetVideoSources{})
	if err != nil {
		log.Fatalf("GetVideoSources failed: %v", err)
	}

	videoSourceToken := vsResp.VideoSources.Token
	fmt.Printf("  Video Source Token: %s\n", videoSourceToken)
	fmt.Printf("  Framerate        : %.1f fps\n", vsResp.VideoSources.Framerate)
	fmt.Printf("  Resolution       : %dx%d\n",
		vsResp.VideoSources.Resolution.Width,
		vsResp.VideoSources.Resolution.Height)
	fmt.Println()

	// ─── 2. GetImagingSettings ──────────────────────────────────────────
	// Retrieves the current image processing settings for a video source.
	// These control how the camera processes the raw sensor data.
	fmt.Println("--- 2. Current Imaging Settings ---")
	settings := getImagingSettings(dev, videoSourceToken)

	fmt.Printf("  Brightness         : %.1f\n", settings.Brightness)
	fmt.Printf("  Contrast           : %.1f\n", settings.Contrast)
	fmt.Printf("  Sharpness          : %.1f\n", settings.Sharpness)
	fmt.Printf("  Color Saturation   : %.1f\n", settings.ColorSaturation)
	fmt.Printf("  IR Cut Filter      : %s\n", settings.IrCutFilter)
	fmt.Printf("  Backlight Comp.    : Mode=%s, Level=%.1f\n",
		settings.BacklightCompensation.Mode, settings.BacklightCompensation.Level)
	fmt.Printf("  Wide Dynamic Range : Mode=%s, Level=%.1f\n",
		settings.WideDynamicRange.Mode, settings.WideDynamicRange.Level)
	fmt.Printf("  White Balance      : Mode=%s\n", settings.WhiteBalance.Mode)
	fmt.Println()

	// ─── 3. Interactive Brightness Adjustment ───────────────────────────
	// Demonstrates SetImagingSettings by allowing the user to adjust
	// brightness up or down by 10 units.
	fmt.Println("--- 3. Adjust Brightness ---")
	fmt.Println("  [+] Increase brightness by 10")
	fmt.Println("  [-] Decrease brightness by 10")
	fmt.Println("  [s] Show current settings")
	fmt.Println("  [q] Quit")
	fmt.Println()

	currentBrightness := settings.Brightness
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("Imaging> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		switch input {
		case "+":
			currentBrightness += 10
			setBrightness(dev, videoSourceToken, currentBrightness)
			fmt.Printf("  Brightness set to %.1f\n", currentBrightness)

		case "-":
			currentBrightness -= 10
			setBrightness(dev, videoSourceToken, currentBrightness)
			fmt.Printf("  Brightness set to %.1f\n", currentBrightness)

		case "s":
			updated := getImagingSettings(dev, videoSourceToken)
			fmt.Printf("  Brightness: %.1f | Contrast: %.1f | Sharpness: %.1f\n",
				updated.Brightness, updated.Contrast, updated.Sharpness)

		case "q", "Q":
			fmt.Println("  Exiting imaging settings.")
			return

		default:
			fmt.Println("  Unknown command. Enter +, -, s, or q.")
		}
		fmt.Println()
	}
}

// getImagingSettings calls GetImagingSettings and parses the XML response.
func getImagingSettings(dev *goonvif.Device, token onvif.ReferenceToken) imagingSettings {
	req := imaging.GetImagingSettings{
		VideoSourceToken: token,
	}

	resp, err := dev.CallMethod(req)
	if err != nil {
		log.Fatalf("GetImagingSettings failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("GetImagingSettings returned HTTP %d:\n%s", resp.StatusCode, string(body))
	}

	var env soapEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		log.Fatalf("Failed to parse GetImagingSettings response: %v", err)
	}

	return env.Body.GetImagingSettingsResponse.ImagingSettings
}

// setBrightness calls SetImagingSettings to change the brightness value.
// Only the Brightness field is set; other fields are left at zero values
// which the camera interprets as "keep current value" for most vendors.
//
// Note: Some cameras require all fields to be sent. In production,
// you would first GetImagingSettings, modify the desired field, then
// send the full settings back via SetImagingSettings.
func setBrightness(dev *goonvif.Device, token onvif.ReferenceToken, brightness float64) {
	req := imaging.SetImagingSettings{
		VideoSourceToken: token,
		ImagingSettings: onvif.ImagingSettings20{
			Brightness: brightness,
		},
	}

	resp, err := dev.CallMethod(req)
	if err != nil {
		log.Printf("SetImagingSettings failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("SetImagingSettings returned HTTP %d:\n%s", resp.StatusCode, string(body))
	}
}
