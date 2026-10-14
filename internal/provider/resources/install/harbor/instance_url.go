package installharbor

import (
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

// P0's rule for a Harbor instance's URL, and its message for one that breaks it
// (packages/integrations/harbor/shared/src/instance-url.ts in the app): the instance's
// origin in the one spelling P0 stores, `https://`, a lowercase host and, if it has
// one, a port, with nothing after it. The connector sends the robot account's
// credentials to this URL, and P0 fixes it when it creates the instance.

const invalidInstanceUrl = "Enter the instance's address as https://<host> or https://<host>:<port>, in lowercase and with nothing after it, " +
	"such as https://harbor.example.com. Leave out a trailing slash, any path, any username or password, and port 443."

// The plain spelling: no character that could close a string or open an
// interpolation in the Terraform that P0 writes the URL into, and no IPv6 literal.
var instanceUrlRegex = regexp.MustCompile(`^https://([a-z0-9.-]+)(?::(\d+))?$`)

// P0 also requires the URL to be its own origin: `new URL(url).origin === url`, by the
// WHATWG URL Standard, which Go's net/url doesn't follow. For a URL that matches the
// pattern above, that parser writes the origin differently in two cases only, so each
// is written out here. It reads the port as a number, so it drops port 443 and a
// port's leading zeros, and fails on a port above 65535. And it reads a host whose
// last label is a number as an IPv4 address, which it writes as four decimal numbers,
// and fails on one that isn't an address. Any other host it leaves as it is.
var (
	// A label the parser reads as a number, in decimal, octal or hexadecimal. The
	// pattern above admits lowercase hexadecimal only.
	numericLabelRegex = regexp.MustCompile(`^(?:[0-9]+|0x[0-9a-f]*)$`)
)

const httpsPort = 443

// Reports whether url is a Harbor instance's address in the one spelling P0 stores.
// Unlike P0, it doesn't ignore whitespace around url: P0 would store the URL without
// it, which would read back different from the configuration.
func isInstanceUrl(url string) bool {
	match := instanceUrlRegex.FindStringSubmatch(url)
	return match != nil && isOriginHost(match[1]) && isOriginPort(match[2])
}

// Whether the parser writes host as it is. One trailing dot leaves a host's last
// label in place, so the parser looks at the label before it.
func isOriginHost(host string) bool {
	labels := strings.Split(strings.TrimSuffix(host, "."), ".")
	if !numericLabelRegex.MatchString(labels[len(labels)-1]) {
		return true
	}
	// The parser writes an address as netip does: four decimal numbers, none with a
	// leading zero.
	address, err := netip.ParseAddr(host)
	return err == nil && address.Is4()
}

// Whether the parser writes port as it is. An empty port is no port.
func isOriginPort(port string) bool {
	if port == "" {
		return true
	}
	if len(port) > 1 && port[0] == '0' {
		return false
	}
	// 16 bits: a port is at most 65535.
	number, err := strconv.ParseUint(port, 10, 16)
	return err == nil && number != httpsPort
}
