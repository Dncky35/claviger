package network

import (
	"fmt"
	"net"
)

// VerificationStatus defines the result of our endpoint check
type VerificationStatus int

const (
	StatusValidIP VerificationStatus = iota
	StatusExactMatch
	StatusMismatch
	StatusNoRecord
)

// VerifyEndpoint checks if the input is an IP or a domain.
// If it's a domain, it queries the internet to see if it points to this server.
func VerifyEndpoint(input string, serverPublicIP string) (VerificationStatus, string) {
	// 1. Is it just a raw IP address?
	if ip := net.ParseIP(input); ip != nil {
		return StatusValidIP, "Valid IP address provided."
	}

	// 2. It's a domain. Let's ask the internet for the A-Record.
	ips, err := net.LookupIP(input)
	if err != nil {
		return StatusNoRecord, fmt.Sprintf("⚠️ Domain '%s' does not exist or has no A-Record pointing to an IP.", input)
	}

	// 3. Does the domain point to OUR server?
	for _, ip := range ips {
		if ip.String() == serverPublicIP {
			return StatusExactMatch, "✅ Domain perfectly matches this server's public IP!"
		}
	}

	// 4. Mismatch (DNS is pointing somewhere else)
	resolvedList := ""
	for i, ip := range ips {
		if i > 0 {
			resolvedList += ", "
		}
		resolvedList += ip.String()
	}

	warning := fmt.Sprintf("⚠️ WARNING: '%s' resolves to [%s], but this server's IP is [%s].\nIf you just updated Cloudflare, DNS propagation can take 10 minutes.", input, resolvedList, serverPublicIP)

	return StatusMismatch, warning
}
