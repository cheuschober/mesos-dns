package records

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mesosphere/mesos-dns/logging"
)

// Config holds mesos dns configuration
type Config struct {
	//  Domain: name of the domain used (default "mesos", ie .mesos domain)
	Domain string
	// EnforceRFC952 will enforce an older, more strict set of rules for DNS labels
	EnforceRFC952 bool
	// File is the location of the config.json file
	File string
	// IPSources is the prioritized list of task IP sources
	IPSources []string // e.g. ["host", "docker", "mesos", "rkt"]
	// Mesos master(s): a list of IP:port pairs for one or more Mesos masters
	Masters []string
	// Refresh frequency: the frequency in seconds of regenerating records (default 60)
	RefreshSeconds int
	// Which backend resolvers to load (builtin by default)
	Resolvers map[string]interface{}
	// Timeout in seconds waiting for the master to return data from StateJson
	StateTimeoutSeconds int
	// SOA record fields (see http://tools.ietf.org/html/rfc1035#page-18)
	SOAExpire  uint32 // expiration time
	SOAMinttl  uint32 // minimum TTL
	SOAMname   string // primary name server
	SOARefresh uint32 // refresh interval
	SOARetry   uint32 // retry interval
	SOARname   string // email of admin esponsible
	SOASerial  uint32 // initial version number (incremented on refresh)
	// Zookeeper: a single Zk url
	Zk string
	// Zookeeper Detection Timeout: how long in seconds to wait for Zookeeper to
	// be initially responsive. Default is 30 and 0 means no timeout.
	ZkDetectionTimeout int
}

// NewConfig return the default config of the resolver
func NewConfig() *Config {
	return &Config{
		Domain:              "mesos",
		IPSources:           []string{"netinfo", "mesos", "host"},
		RefreshSeconds:      60,
		StateTimeoutSeconds: 300,
		SOAExpire:           86400,
		SOAMinttl:           60,
		SOAMname:            "ns1.mesos",
		SOARefresh:          60,
		SOARetry:            600,
		SOARname:            "root.ns1.mesos",
		SOASerial:           uint32(time.Now().Unix()),
		ZkDetectionTimeout:  30,
	}
}

// SetConfig instantiates a Config struct read in from config.json
func SetConfig(cjson string) *Config {
	c, err := ReadConfig(cjson)
	if err != nil {
		logging.Error.Fatal(err)
	}
	logging.Verbose.Printf("config loaded from %q", c.File)

	// validate and complete configuration file
	if err = validateMasters(c.Masters); err != nil {
		logging.Error.Fatalf("Masters validation failed: %v", err)
	}

	if err = validateIPSources(c.IPSources); err != nil {
		logging.Error.Fatalf("IPSources validation failed: %v", err)
	}

	// Default to builtin config if none have been specified
	if c.Resolvers == nil {
		c.Resolvers = map[string]interface{}{"builtin": nil}
	}

	c.Domain = strings.ToLower(c.Domain)

	// SOA record fields
	c.SOARname = strings.TrimRight(strings.Replace(c.SOARname, "@", ".", -1), ".") + "."
	c.SOAMname = strings.TrimRight(c.SOAMname, ".") + "."

	// print the configuration file
	logging.Verbose.Println("Mesos-DNS configuration:")
	logging.Verbose.Println("   - ConfigFile: ", c.File)
	logging.Verbose.Println("   - Domain: " + c.Domain)
	logging.Verbose.Println("   - EnforceRFC952: ", c.EnforceRFC952)
	logging.Verbose.Println("   - IPSources: ", c.IPSources)
	logging.Verbose.Println("   - Masters: " + strings.Join(c.Masters, ", "))
	logging.Verbose.Println("   - RefreshSeconds: ", c.RefreshSeconds)
	logging.Verbose.Println("   - SOAMname: " + c.SOAMname)
	logging.Verbose.Println("   - SOARname: " + c.SOARname)
	logging.Verbose.Println("   - SOASerial: ", c.SOASerial)
	logging.Verbose.Println("   - SOARefresh: ", c.SOARefresh)
	logging.Verbose.Println("   - SOARetry: ", c.SOARetry)
	logging.Verbose.Println("   - SOAExpire: ", c.SOAExpire)
	logging.Verbose.Println("   - StateTimeoutSeconds: ", c.StateTimeoutSeconds)
	logging.Verbose.Println("   - Zookeeper: ", c.Zk)
	logging.Verbose.Println("   - ZookeeperDetectionTimeout: ", c.ZkDetectionTimeout)

	// print individual configurations for resolvers
	logging.Verbose.Println("   - Resolvers:")
	for k, v := range c.Resolvers {
		logging.Verbose.Printf("     - %s:\n", k)
		if m, ok := v.(map[string]interface{}); ok {
			for key, val := range m {
				logging.Verbose.Printf("       - %s: %+v\n", key, val)
			}
		}
	}
	return c
}

func ReadConfig(file string) (*Config, error) {
	var err error

	workingDir := "."
	for _, name := range []string{"HOME", "USERPROFILE"} { // *nix, windows
		if dir := os.Getenv(name); dir != "" {
			workingDir = dir
		}
	}

	c := NewConfig()
	// Check file path, read file, Unmarshal JSON
	file, err = filepath.Abs(strings.Replace(file, "~/", workingDir+"/", 1))
	if err != nil {
		return nil, fmt.Errorf("cannot find configuration file")
	} else if bs, err := ioutil.ReadFile(file); err != nil {
		return nil, fmt.Errorf("missing configuration file: %q", file)
	} else if err = json.Unmarshal(bs, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config file %q: %v", file, err)
	}

	return c, nil
}

func unique(ss []string) []string {
	set := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ok := set[s]; !ok {
			set[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}
