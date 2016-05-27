package consul

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"

	capi "github.com/hashicorp/consul/api"
	"github.com/mesos/mesos-go/upid"
	"github.com/mesosphere/mesos-dns/logging"
	"github.com/mesosphere/mesos-dns/records"
	"github.com/mesosphere/mesos-dns/records/state"
)

type ConsulBackend struct {
	Agents           map[string]*capi.Agent
	AgentPort        string
	Client           *capi.Client
	Config           *Config
	Count            int
	IPSources        []string
	Refresh          int
	ServicePrefix    string
	SlaveIDIP        map[string]string
	SlaveIPID        map[string]string
	SlaveIDHostname  map[string]string
	SlaveHostnameID  map[string]string
	State            state.State
	FrameworkRecords map[string]*ConsulRecords
	MesosRecords     map[string]*ConsulRecords
	TaskRecords      map[string]*ConsulRecords
	HealthChecks     map[string]*ConsulChecks
	ConsulServices   map[string]*ConsulRecords
}

type ConsulRecords struct {
	Current  []*capi.AgentServiceRegistration
	Previous []*capi.AgentServiceRegistration
}

type ConsulChecks struct {
	Current  []*capi.AgentCheckRegistration
	Previous []*capi.AgentCheckRegistration
}

type TaskCheck struct {
	TaskID string
	*capi.AgentCheckRegistration
}

func New(config *Config, errch chan error, rg *records.RecordGenerator, version string) *ConsulBackend {
	cfg := capi.DefaultConfig()
	cfg.Address = config.Address
	cfg.Datacenter = config.Datacenter
	cfg.Scheme = config.Scheme
	cfg.Token = config.Token

	client, err := capi.NewClient(cfg)
	if err != nil {
		errch <- err
		return &ConsulBackend{}
	}

	if config.CacheRefresh <= 0 {
		errch <- errors.New("Cache refresh must be greater than 0")
		return &ConsulBackend{}
	}

	// Since the consul api is dumb and wont return the http port
	// we'll assume all agents are running on the same port as
	// the initially specified consul server
	port := strings.Split(config.Address, ":")[1]

	return &ConsulBackend{
		Agents:           make(map[string]*capi.Agent),
		AgentPort:        port,
		Client:           client,
		Config:           config,
		Count:            0,
		IPSources:        rg.Config.IPSources,
		Refresh:          rg.Config.RefreshSeconds,
		ServicePrefix:    "mesos-dns",
		SlaveIDIP:        make(map[string]string),
		SlaveIPID:        make(map[string]string),
		SlaveIDHostname:  make(map[string]string),
		SlaveHostnameID:  make(map[string]string),
		State:            state.State{},
		FrameworkRecords: make(map[string]*ConsulRecords),
		MesosRecords:     make(map[string]*ConsulRecords),
		TaskRecords:      make(map[string]*ConsulRecords),
		HealthChecks:     make(map[string]*ConsulChecks),
		ConsulServices:   make(map[string]*ConsulRecords),
	}
}

func (c *ConsulBackend) Reload(rg *records.RecordGenerator) {
	c.Count++
	// Get a snapshot of state.json
	c.State = rg.State

	// Get agent members
	// and initialize client connections
	err := c.connectAgents()
	if err != nil {
		logging.Error.Println("Failed to connect to consul agent", err)
		return
	}

	// Generate all of the necessary records
	c.generateMesosRecords()
	c.generateFrameworkRecords()
	for _, framework := range rg.State.Frameworks {
		c.generateTaskRecords(framework.Tasks)
	}

	// Register records with consul
	c.Register()

	// Clean up old/orphaned records
	c.Cleanup()
}

func (c *ConsulBackend) connectAgents() error {
	// Only get LAN members
	members, err := c.Client.Agent().Members(false)
	if err != nil {
		// Do something
		return err
	}

	for _, agent := range members {
		logging.VeryVerbose.Println("Connecting to consul agent", agent)
		// Test connection to each agent and reconnect as needed
		if _, ok := c.Agents[agent.Addr]; ok {
			_, err := c.Agents[agent.Addr].Self()
			if err == nil {
				logging.VeryVerbose.Println("Connected to consul agent", agent)
				continue
			}
		}
		cfg := capi.DefaultConfig()
		cfg.Address = agent.Addr + ":" + c.AgentPort
		cfg.Datacenter = c.Config.Datacenter
		cfg.Scheme = c.Config.Scheme
		cfg.Token = c.Config.Token

		client, err := capi.NewClient(cfg)
		if err != nil {
			// How do we want to handle consul agent not being responsive
			return err
		}

		// Do a sanity check that we are connected to agent
		_, err = client.Agent().Self()
		if err != nil {
			// Dump agent?
			return err
		}

		c.Agents[agent.Name] = client.Agent()
		logging.VeryVerbose.Println("Connected to consul agent", agent)
	}

	return nil
}

func (c *ConsulBackend) generateMesosRecords() {
	// Create a bogus Slave struct for the leader
	// master@10.10.10.8:5050
	lead := state.Slave{
		PID: state.PID{
			&upid.UPID{
				Host: strings.Split(strings.Split(c.State.Leader, "@")[1], ":")[0],
				Port: strings.Split(strings.Split(c.State.Leader, "@")[1], ":")[1],
			},
		},
	}

	for _, slave := range c.State.Slaves {
		// We'll need this for service registration to the appropriate
		// slaves
		c.SlaveIDIP[slave.ID] = slave.PID.Host
		c.SlaveIPID[slave.PID.Host] = slave.ID

		// Pull out only the hostname, not the FQDN
		c.SlaveIDHostname[slave.ID] = strings.Split(slave.Hostname, ".")[0]
		c.SlaveHostnameID[strings.Split(slave.Hostname, ".")[0]] = slave.ID

		tags := []string{"slave", c.SlaveIDHostname[slave.ID]}

		if slave.Attrs.Master == "true" {
			tags = append(tags, "master")
		}

		if slave.PID.Host == lead.PID.Host {
			tags = append(tags, "leader")
		}

		port, err := strconv.Atoi(slave.PID.Port)
		if err != nil {
			logging.Error.Println("Failed to get port for slave", slave.ID, ". Error:", err)
			continue
		}

		// Make sure we've got everything initialized
		if _, ok := c.MesosRecords[slave.ID]; !ok {
			c.MesosRecords[slave.ID] = &ConsulRecords{}
		}

		c.MesosRecords[slave.ID].Current = append(c.MesosRecords[slave.ID].Current, createService(strings.Join([]string{c.ServicePrefix, slave.ID}, ":"), "mesos", slave.PID.Host, port, tags))
		for _, rec := range c.MesosRecords[slave.ID].Current {
			logging.VeryVerbose.Println("Mesos records:", rec.ID, "=>", rec.Address)
		}

	}
	// This is gonna be wrong when current + previous is included I think
	logging.Verbose.Println("Found", len(c.MesosRecords), "mesos records")
}

func (c *ConsulBackend) generateFrameworkRecords() {
	for _, framework := range c.State.Frameworks {
		// Skip inactive frameworks
		if !framework.Active {
			continue
		}

		// task, pid, name, hostname
		port, err := strconv.Atoi(framework.PID.Port)
		if err != nil {
			logging.Error.Println("Failed to get port for framework", framework.Name, ". Error:", err)
			continue
		}

		// Silly
		var slaveid string
		if _, ok := c.SlaveIPID[framework.PID.Host]; ok {
			slaveid = c.SlaveIPID[framework.PID.Host]
		} else {
			// Try to discover the slave that the framework is running on
			// by comparing the hostnames
			for id, hostname := range c.SlaveIDHostname {
				if hostname == strings.Split(framework.Hostname, ".")[0] {
					slaveid = id
				}
			}
		}

		if slaveid == "" {
			logging.Error.Println("Unable to find slave for", framework.Name)
			continue
		}

		// Make sure we've got everything initialized
		if _, ok := c.FrameworkRecords[slaveid]; !ok {
			c.FrameworkRecords[slaveid] = &ConsulRecords{}
		}

		c.FrameworkRecords[slaveid].Current = append(c.FrameworkRecords[slaveid].Current, createService(strings.Join([]string{c.ServicePrefix, framework.Name}, ":"), framework.Name, framework.PID.Host, port, []string{}))
		for _, rec := range c.FrameworkRecords[slaveid].Current {
			logging.VeryVerbose.Println("Framework record:", rec.ID, "=>", rec.Address)
		}
		logging.Verbose.Println("Found", len(c.FrameworkRecords), "framework records on slave", slaveid)
	}
}

func (c *ConsulBackend) generateTaskRecords(tasks []state.Task) {
	for _, task := range tasks {
		if task.State != "TASK_RUNNING" {
			continue
		}
		// Might be able to use task.SlaveIP, but in testing this returns ""
		address := c.getAddress(task)

		// Make sure we've got everything initialized
		if _, ok := c.TaskRecords[task.SlaveID]; !ok {
			c.TaskRecords[task.SlaveID] = &ConsulRecords{}
		}
		if _, ok := c.HealthChecks[task.SlaveID]; !ok {
			c.HealthChecks[task.SlaveID] = &ConsulChecks{}
		}

		// Create a service registration for every port
		if len(task.Ports()) == 0 {
			// Create a service registration for the base service
			id := strings.Join([]string{c.ServicePrefix, c.SlaveIDHostname[task.SlaveID], task.ID}, ":")
			c.TaskRecords[task.SlaveID].Current = append(c.TaskRecords[task.SlaveID].Current, createService(id, task.Name, address, 0, []string{c.SlaveIDHostname[task.SlaveID]}))
			c.HealthChecks[task.SlaveID].Current = append(c.HealthChecks[task.SlaveID].Current, c.getHealthChecks(task, id, address, 0)...)
		} else {
			// Create a service registration for each port
			for _, port := range task.Ports() {
				p, err := strconv.Atoi(port)
				if err != nil {
					logging.Error.Println("Failed to get port for task", task.ID, ", Skipping. Error:", err)
					continue
				}
				id := strings.Join([]string{c.ServicePrefix, c.SlaveIDHostname[task.SlaveID], task.ID, port}, ":")
				c.TaskRecords[task.SlaveID].Current = append(c.TaskRecords[task.SlaveID].Current, createService(id, task.Name, address, p, []string{c.SlaveIDHostname[task.SlaveID]}))
				c.HealthChecks[task.SlaveID].Current = append(c.HealthChecks[task.SlaveID].Current, c.getHealthChecks(task, id, address, p)...)
			}
		}
		for _, rec := range c.TaskRecords[task.SlaveID].Current {
			logging.VeryVerbose.Println("Task record", rec.ID, "=>", rec.Address)
		}
		for _, rec := range c.HealthChecks[task.SlaveID].Current {
			logging.VeryVerbose.Println("Health check record", rec.ID, "=>", rec.Name, "/", rec.ServiceID)
		}
		logging.Verbose.Println("Found", len(c.TaskRecords[task.SlaveID].Current), "task records on slave", task.SlaveID)
		logging.Verbose.Println("Found", len(c.HealthChecks[task.SlaveID].Current), "health check records on slave", task.SlaveID)
	}
}

func (c *ConsulBackend) Register() {

	var wg sync.WaitGroup

	for agentid, _ := range c.Agents {
		// Launch a goroutine per agent
		slaveid := c.SlaveHostnameID[agentid]

		wg.Add(1)
		go func() {
			services := []*capi.AgentServiceRegistration{}

			// May not have any records for specific slave
			if _, ok := c.MesosRecords[slaveid]; ok {
				services = append(services, getDeltaServices(c.MesosRecords[slaveid].Previous, c.MesosRecords[slaveid].Current)...)
			}
			if _, ok := c.FrameworkRecords[slaveid]; ok {
				services = append(services, getDeltaServices(c.FrameworkRecords[slaveid].Previous, c.FrameworkRecords[slaveid].Current)...)
			}
			if _, ok := c.TaskRecords[slaveid]; ok {
				services = append(services, getDeltaServices(c.TaskRecords[slaveid].Previous, c.TaskRecords[slaveid].Current)...)
			}

			for _, service := range services {
				err := c.Agents[agentid].ServiceRegister(service)
				if err != nil {
					logging.Error.Println("Failed to register service", service.ID, "on agent", agentid, ". Error:", err)
					continue
				}

				if _, ok := c.HealthChecks[slaveid]; !ok {
					logging.VeryVerbose.Println("No healthchecks for", service.ID, "on slave", slaveid)
					continue
				}

			}

			checks := []*capi.AgentCheckRegistration{}
			if _, ok := c.HealthChecks[slaveid]; ok {
				checks = append(checks, getDeltaChecks(c.HealthChecks[slaveid].Previous, c.HealthChecks[slaveid].Current, "add")...)
			}

			for _, hc := range checks {
				logging.VeryVerbose.Println("Registering check", hc.Name, "for service", hc.ServiceID)
				err := c.Agents[agentid].CheckRegister(hc)
				if err != nil {
					logging.Error.Println("Failed to register healthcheck for service", hc.ServiceID, ". Error:", err)
					continue
				}
			}
			wg.Done()
		}()
		wg.Wait()
	}
}

func (c *ConsulBackend) Cleanup() {

	var wg sync.WaitGroup

	for agentid, _ := range c.Agents {
		// Launch a goroutine per agent
		slaveid := c.SlaveHostnameID[agentid]
		wg.Add(1)
		go func() {
			services := []*capi.AgentServiceRegistration{}
			currentservices := []*capi.AgentServiceRegistration{}

			// May not have any records for specific slave
			if _, ok := c.MesosRecords[slaveid]; ok {
				services = append(services, getDeltaServices(c.MesosRecords[slaveid].Current, c.MesosRecords[slaveid].Previous)...)
				currentservices = append(currentservices, c.MesosRecords[slaveid].Current...)
				c.MesosRecords[slaveid].Previous = c.MesosRecords[slaveid].Current
				c.MesosRecords[slaveid].Current = nil
			}
			if _, ok := c.FrameworkRecords[slaveid]; ok {
				services = append(services, getDeltaServices(c.FrameworkRecords[slaveid].Current, c.FrameworkRecords[slaveid].Previous)...)
				currentservices = append(currentservices, c.FrameworkRecords[slaveid].Current...)
				c.FrameworkRecords[slaveid].Previous = c.FrameworkRecords[slaveid].Current
				c.FrameworkRecords[slaveid].Current = nil
			}
			if _, ok := c.TaskRecords[slaveid]; ok {
				services = append(services, getDeltaServices(c.TaskRecords[slaveid].Current, c.TaskRecords[slaveid].Previous)...)
				currentservices = append(currentservices, c.TaskRecords[slaveid].Current...)
				c.TaskRecords[slaveid].Previous = c.TaskRecords[slaveid].Current
				c.TaskRecords[slaveid].Current = nil
			}

			services = append(services, getDeltaServices(currentservices, c.getServices(agentid))...)

			for _, service := range services {
				logging.VeryVerbose.Println("Removing service", service.ID, "on agent", agentid)
				err := c.Agents[agentid].ServiceDeregister(service.ID)
				if err != nil {
					logging.Verbose.Println("Failed to deregister service", service.ID, "on agent", agentid, ". Falling back to catalog deregistration.")
					logging.Error.Println("Error:", err)
					dereg := &capi.CatalogDeregistration{
						Node:      c.SlaveIDHostname[slaveid],
						ServiceID: service.ID,
					}
					_, err = c.Client.Catalog().Deregister(dereg, nil)
					if err != nil {
						logging.Error.Println("Failed to deregister service from catalog", err)
						continue
					}
				}
			}
			checks := []*capi.AgentCheckRegistration{}
			if _, ok := c.HealthChecks[slaveid]; ok {
				checks = append(checks, getDeltaChecks(c.HealthChecks[slaveid].Current, c.HealthChecks[slaveid].Previous, "purge")...)
				c.HealthChecks[slaveid].Previous = c.HealthChecks[slaveid].Current
				c.HealthChecks[slaveid].Current = nil
			}

			for _, hc := range checks {
				logging.VeryVerbose.Println("Removing", hc.ID)
				err := c.Agents[agentid].CheckDeregister(hc.ID)
				if err != nil {
					logging.Verbose.Println("Failed to deregister check", hc.ID, "on agent", agentid, " . Falling back to catalog deregistration.")
					logging.Error.Println("Error:", err)
					dereg := &capi.CatalogDeregistration{
						Node:    c.SlaveIDHostname[slaveid],
						CheckID: hc.ID,
					}
					_, err = c.Client.Catalog().Deregister(dereg, nil)
					if err != nil {
						logging.Error.Println("Failed to deregister check", hc.ID, "from catalog", err)
						continue
					}
				}
			}
			wg.Done()
		}()
		wg.Wait()
	}
}

func (c *ConsulBackend) getAddress(task state.Task) string {

	var address string
	for _, lookup := range c.IPSources {
		lookupkey := strings.Split(lookup, ":")
		switch lookupkey[0] {
		case "mesos":
			address = task.IP("mesos")
		case "docker":
			address = task.IP("docker")
		case "netinfo":
			address = task.IP("netinfo")
		case "host":
			address = task.IP("host")
		case "label":
			if len(lookupkey) != 2 {
				logging.Error.Fatal("Lookup order label is not in proper format `label:labelname`")
			}
			addresses := state.StatusIPs(task.Statuses, state.Labels(lookupkey[1]))
			if len(addresses) > 0 {
				address = addresses[0]
			}
		}

		if address != "" {
			break
		}
	}

	// If still empty, set to host IP
	if address == "" {
		address = c.SlaveIDIP[task.SlaveID]
	}

	return address
}

func (c *ConsulBackend) getHealthChecks(task state.Task, id string, address string, port int) []*capi.AgentCheckRegistration {
	// Look up KV for defined healthchecks
	kv := c.Client.KV()
	var hc []*capi.AgentCheckRegistration
	for _, label := range task.Labels {
		if label.Key != "ConsulHealthCheckKeys" {
			continue
		}
		for _, endpoint := range strings.Split(label.Value, ",") {
			pair, _, err := kv.Get("healthchecks/"+endpoint, nil)
			// If key does not exist in consul, pair is nil
			if pair == nil || err != nil {
				logging.Verbose.Println("Healthcheck healthchecks/"+endpoint, "not found in consul, skipping")
				continue
			}
			check := &capi.AgentCheckRegistration{}
			err = json.Unmarshal(pair.Value, check)
			if err != nil {
				logging.Error.Println(err)
				continue
			}
			check.ID = strings.Join([]string{check.ID, id}, ":")
			check.ServiceID = id

			// Now to do some variable expansion
			// We're going to reserve `{IP}` and `{PORT}`
			// We'll apply this to
			// http, script, tcp
			if check.HTTP != "" {
				if !evalVars(&check.HTTP, address, port) {
					continue
				}
			}
			if check.Script != "" {
				if !evalVars(&check.Script, address, port) {
					continue
				}
			}
			if check.TCP != "" {
				if !evalVars(&check.TCP, address, port) {
					continue
				}
			}

			hc = append(hc, check)
		}
	}
	logging.VeryVerbose.Println("Found", len(hc), "healthchecks for", id)

	return hc

}

func (c *ConsulBackend) getServices(agentid string) []*capi.AgentServiceRegistration {
	svcs := []*capi.AgentServiceRegistration{}
	if c.Config.CacheOnly {
		return svcs
	}

	if c.Count%c.Config.CacheRefresh != 0 {
		return svcs
	}

	// Reset the counter so we dont overflow
	c.Count = 0
	services, err := c.Agents[agentid].Services()
	if err != nil {
		logging.Error.Println("Failed to get list of services from consul@", agentid, ".", err)
		return svcs
	}

	for _, service := range services {
		// Filter only services we care about
		if strings.Split(service.ID, ":")[0] != c.ServicePrefix {
			continue
		}

		// Create ServiceRegistration structs for each service so we can compare later
		sr := &capi.AgentServiceRegistration{
			ID:      service.ID,
			Name:    service.Service,
			Tags:    service.Tags,
			Port:    service.Port,
			Address: service.Address,
		}
		// Make sure we've got everything initialized
		svcs = append(svcs, sr)
	}

	return svcs
}

func createService(id string, name string, address string, port int, tags []string) *capi.AgentServiceRegistration {
	// Format the name appropriately
	reg, err := regexp.Compile("[^\\w-]")
	if err != nil {
		logging.Error.Println(err)
		return &capi.AgentServiceRegistration{}
	}

	s := reg.ReplaceAllString(name, "-")

	asr := &capi.AgentServiceRegistration{
		ID:      id,
		Name:    strings.ToLower(strings.Replace(s, "_", "", -1)),
		Address: address,
		Tags:    tags,
	}
	if port > 0 {
		asr.Port = port
	}

	return asr
}

func compareService(newservice *capi.AgentServiceRegistration, oldservice *capi.AgentServiceRegistration) bool {
	// Do this explicitly cause #grumpyface
	if newservice.ID != oldservice.ID {
		return false
	}
	if newservice.Name != oldservice.Name {
		return false
	}
	if newservice.Address != oldservice.Address {
		return false
	}
	if newservice.Port != oldservice.Port {
		return false
	}
	if len(newservice.Tags) != len(oldservice.Tags) {
		return false
	}

	for _, otag := range oldservice.Tags {
		found := false
		for _, ntag := range newservice.Tags {
			if otag == ntag {
				found = true
			}
		}

		if !found {
			return false
		}
	}
	return true
}

func compareCheck(newcheck *capi.AgentCheckRegistration, oldcheck *capi.AgentCheckRegistration) bool {
	if newcheck.ID != oldcheck.ID {
		return false
	}
	if newcheck.Name != oldcheck.Name {
		return false
	}
	if newcheck.ServiceID != oldcheck.ServiceID {
		return false
	}
	if newcheck.AgentServiceCheck != oldcheck.AgentServiceCheck {
		return false
	}
	return true
}

func getDeltaServices(oldservices []*capi.AgentServiceRegistration, newservices []*capi.AgentServiceRegistration) []*capi.AgentServiceRegistration {
	delta := []*capi.AgentServiceRegistration{}
	// Need to compare current vs old
	for _, service := range newservices {
		found := false
		for _, existing := range oldservices {
			if compareService(service, existing) {
				found = true
				break
			}
		}
		if !found {
			delta = append(delta, service)
		}
	}
	return delta
}

func getDeltaChecks(oldchecks []*capi.AgentCheckRegistration, newchecks []*capi.AgentCheckRegistration, context string) []*capi.AgentCheckRegistration {
	delta := []*capi.AgentCheckRegistration{}
	for _, newhc := range newchecks {
		found := false
		for _, oldhc := range oldchecks {
			if compareCheck(newhc, oldhc) {
				found = true
				break
			}
			// We add this context key in here to know how to react to IDs that are the same
			// In a registration/add case, we want to update the healthcheck if it is different
			// even if it uses the same ID
			// In a deregistration/purge case, we want to leave the healthcheck alone if the IDs are
			// the same but the content differs. This is so we don't remove a newly updated healthcheck
			// that uses the same ID as the old one
			if context == "purge" {
				if newhc.ID == oldhc.ID {
					found = true
					break
				}
			}
		}
		if !found {
			delta = append(delta, newhc)
		}
	}
	return delta
}

func evalVars(check *string, address string, port int) bool {
	if strings.Contains(*check, "{PORT}") && port == 0 {
		logging.Error.Println("Invalid port for substitution in healthcheck", *check, port)
		return false
	}
	*check = strings.Replace(*check, "{IP}", address, -1)
	*check = strings.Replace(*check, "{PORT}", strconv.Itoa(port), -1)
	return true
}
