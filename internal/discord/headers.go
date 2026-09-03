package discord

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

// The browser the tool imitates. Every value below is generated in this one
// place; ClientBuildNumber is the single constant to bump when Discord's web
// client moves on.
const (
	ClientBuildNumber = 523497
	chromeVersion     = "146.0.0.0"
	chromeMajor       = "146"
	userAgent         = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + chromeVersion + " Safari/537.36"
)

// superProperties is the X-Super-Properties JSON, in the key order the Node
// client used.
type superProperties struct {
	OS                     string  `json:"os"`
	Browser                string  `json:"browser"`
	Device                 string  `json:"device"`
	SystemLocale           string  `json:"system_locale"`
	HasClientMods          bool    `json:"has_client_mods"`
	BrowserUserAgent       string  `json:"browser_user_agent"`
	BrowserVersion         string  `json:"browser_version"`
	OSVersion              string  `json:"os_version"`
	Referrer               string  `json:"referrer"`
	ReferringDomain        string  `json:"referring_domain"`
	ReferrerCurrent        string  `json:"referrer_current"`
	ReferringDomainCurrent string  `json:"referring_domain_current"`
	ReleaseChannel         string  `json:"release_channel"`
	ClientBuildNumber      int     `json:"client_build_number"`
	ClientEventSource      *string `json:"client_event_source"`
}

var encodedSuperProperties = func() string {
	b, err := json.Marshal(superProperties{
		OS:                "Mac OS X",
		Browser:           "Chrome",
		SystemLocale:      "en-US",
		BrowserUserAgent:  userAgent,
		BrowserVersion:    chromeVersion,
		OSVersion:         "10.15.7",
		ReleaseChannel:    "stable",
		ClientBuildNumber: ClientBuildNumber,
	})
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}()

// Headers returns the full header set for one request.
func Headers(token, timezone string) http.Header {
	h := http.Header{}
	h.Set("Authorization", token)
	h.Set("User-Agent", userAgent)
	h.Set("X-Super-Properties", encodedSuperProperties)
	h.Set("X-Discord-Locale", "en-US")
	h.Set("X-Discord-Timezone", timezone)
	h.Set("X-Debug-Options", "bugReporterEnabled")
	h.Set("Sec-Ch-Ua", `"Chromium";v="`+chromeMajor+`", "Not-A.Brand";v="24", "Google Chrome";v="`+chromeMajor+`"`)
	h.Set("Sec-Ch-Ua-Mobile", "?0")
	h.Set("Sec-Ch-Ua-Platform", `"macOS"`)
	h.Set("Referer", "https://discord.com/channels/@me")
	return h
}

// LocalTimezone finds the IANA name of the local time zone the way a browser
// would report it: TZ first, then the /etc/localtime link, then UTC.
func LocalTimezone(getenv func(string) string) string {
	if tz := getenv("TZ"); tz != "" && tz != "Local" {
		return tz
	}
	if link, err := os.Readlink("/etc/localtime"); err == nil {
		if i := strings.Index(link, "zoneinfo/"); i >= 0 {
			return link[i+len("zoneinfo/"):]
		}
	}
	if name := time.Local.String(); name != "" && name != "Local" {
		return name
	}
	return "UTC"
}
