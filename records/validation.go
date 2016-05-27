package records

import (
	"fmt"
	"net"
	"strings"
)

func validateEnabledServices(c *Config) error {
	if len(c.Masters) == 0 && c.Zk == "" {
		return fmt.Errorf("specify mesos masters or zookeeper in config.json")
	}
	return nil
}

// validateMasters checks that each master in the list is a properly formatted host:ip pair.
// duplicate masters in the list are not allowed.
// returns nil if the masters list is empty, or else all masters in the list are valid.
func validateMasters(ms []string) error {
	if len(ms) == 0 {
		return nil
	}
	valid := make(map[string]struct{}, len(ms))
	for i, m := range ms {
		h, p, err := net.SplitHostPort(m)
		if err != nil {
			return fmt.Errorf("illegal host:port specified for master %q", ms[i])
		}
		// normalize ipv6 addresses
		if ip := net.ParseIP(h); ip != nil {
			h = ip.String()
			m = h + "_" + p
		}
		//TODO(jdef) distinguish between intended hostnames and invalid ip addresses
		if _, found := valid[m]; found {
			return fmt.Errorf("duplicate master specified: %v", ms[i])
		}
		valid[m] = struct{}{}
	}
	return nil
}

// validateIPSources checks validity of ip sources
func validateIPSources(srcs []string) error {
	if len(srcs) == 0 {
		return fmt.Errorf("empty ip sources")
	}
	if len(srcs) != len(unique(srcs)) {
		return fmt.Errorf("duplicate ip source specified")
	}
	for _, src := range srcs {
		source := strings.Split(src, ":")
		switch source[0] {
		case "host", "docker", "mesos", "netinfo", "label":
		default:
			return fmt.Errorf("invalid ip source %q", src)
		}
	}

	return nil
}
