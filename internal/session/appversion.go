package session

import "runtime"

// MCPVersion is the version reported in the x-pm-appversion header. Bumping
// this is a deliberate act — Proton may have allowlist behavior keyed on
// specific (platform, client, version) tuples.
const MCPVersion = "0.1.0"

// appVersionHeader returns a Proton-acceptable x-pm-appversion value following
// the proton-bridge convention: <api-os>-<appname>@<version>. Proton parses
// the platform from before the first `-` and validates against an allowlist;
// "macos"/"linux"/"windows" are known-good prefixes.
func appVersionHeader() string {
	return apiOS() + "-protonmail-mcp@" + MCPVersion
}

func apiOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	case "windows":
		return "windows"
	default:
		// linux + anything else falls back to linux. Matches proton-bridge.
		return "linux"
	}
}
