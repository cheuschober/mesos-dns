package resolvers

import (
	"strings"

	"github.com/mesosphere/mesos-dns/logging"
	"github.com/mesosphere/mesos-dns/records"
	"github.com/mesosphere/mesos-dns/resolvers/builtin"
	"github.com/mesosphere/mesos-dns/resolvers/consul"
	"github.com/mesosphere/mesos-dns/utils"
)

type Resolver interface {
	Reload(rg *records.RecordGenerator)
}

func New(errch chan error, rg *records.RecordGenerator, version string) []Resolver {
	var resolvers []Resolver

	for k, v := range rg.Config.Resolvers {
		// Each backend receives their config as type interface{} or nil.
		switch strings.ToLower(k) {
		case "builtin":
			conf := builtin.NewConfig()
			utils.Merge(v, conf)
			// Print out here after the merge to show the merged config
			logging.VeryVerbose.Printf("Initializing resolver %s with config %+v\n", k, conf)
			resolvers = append(resolvers, builtin.New(conf, errch, rg, version))
		case "consul":
			conf := consul.NewConfig()
			utils.Merge(v, conf)
			// Print out here after the merge to show the merged config
			logging.VeryVerbose.Printf("Initializing resolver %s with config %+v\n", k, conf)
			resolvers = append(resolvers, consul.New(conf, errch, rg, version))
		}
	}

	return resolvers
}
