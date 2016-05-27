package builtin

import (
	"fmt"
	"net"
)

func validateEnabledServices(c *Config) error {
	if !c.DNSOn && !c.HTTPOn {
		return fmt.Errorf("Either DNS or HTTP server should be on")
	}
	return nil
}

// validateExternalDNS checks that each remote server's IP in the list is a properly
// formatted IP address. Duplicate IPs in the list are not allowed.
// returns nil if the remote server list is empty, or else all IPs in the list are valid.
func validateExternalDNS(rs []string) error {
	if len(rs) == 0 {
		return nil
	}
	ips := make(map[string]struct{}, len(rs))
	for _, r := range rs {
		ip := net.ParseIP(r)
		if ip == nil {
			return fmt.Errorf("illegal IP specified for remote server %q", r)
		}
		ipstr := ip.String()
		if _, found := ips[ipstr]; found {
			return fmt.Errorf("duplicate remote IP specified: %v", r)
		}
		ips[ipstr] = struct{}{}
	}
	return nil
}
