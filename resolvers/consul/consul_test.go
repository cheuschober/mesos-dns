package consul

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	capi "github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/testutil"
	"github.com/mesosphere/mesos-dns/logging"
	"github.com/mesosphere/mesos-dns/records"
	"github.com/mesosphere/mesos-dns/records/state"
)

const LOCALSLAVEID = "20160107-001256-134875658-5050-27524-S3"
const LOCALSLAVENAME = "mesosslave-r02-s02"
const LOCALSLAVEIP = "127.0.0.1"

func TestNew(t *testing.T) {
	server, _ := backendSetup(t)
	defer server.Stop()
}

func TestConnectAgents(t *testing.T) {
	server, backend := recordSetup(t)
	defer server.Stop()

	// Let's see what happens the second time
	// It should just return early since the agent already
	// exists in our list of agents
	err := backend.connectAgents()
	if err != nil {
		t.Log(err)
	}

	if len(backend.Agents) != 1 {
		t.Error("Failed to get agent connection")
	}

}

func TestMesosRecords(t *testing.T) {
	server, backend := recordSetup(t)
	defer server.Stop()

	backend.generateMesosRecords()

	// Each slave is a single id
	expected := map[string]int{
		"20160107-001256-134875658-5050-27524-S66": 1,
		LOCALSLAVEID:                               1,
		"20160107-001256-134875658-5050-27524-S1":  1,
		"20160107-001256-134875658-5050-27524-S2":  1,
		"20160107-001256-134875658-5050-27524-S0":  1,
	}

	// 6 records ( 5x slaves, 1x leader )
	validateStateRecords(t, backend.MesosRecords, 5, expected)
}

func TestFrameworkRecords(t *testing.T) {
	server, backend := recordSetup(t)
	defer server.Stop()

	backend.generateMesosRecords()
	backend.generateFrameworkRecords()

	// Framework is only running on a single slave
	expected := map[string]int{LOCALSLAVEID: 1}

	// 1 record ( marathon )
	validateStateRecords(t, backend.FrameworkRecords, 1, expected)

	// Set up test for inactive framework
	backend.State.Frameworks[0].Active = false
	backend.FrameworkRecords = make(map[string]*ConsulRecords)
	backend.generateFrameworkRecords()
	expected[LOCALSLAVEID] = 0

	validateStateRecords(t, backend.FrameworkRecords, 0, expected)
}

func TestTaskRecords(t *testing.T) {
	server, backend := recordSetup(t)
	defer server.Stop()

	// Need to do this to populate backend.SlaveIDIP
	// so we can pull appropriate slave ip mapping
	// by slave.ID
	backend.generateMesosRecords()

	for _, framework := range backend.State.Frameworks {
		backend.generateTaskRecords(framework.Tasks)
	}

	// Each slave can have a different number of tasks running
	expected := map[string]int{
		"20160107-001256-134875658-5050-27524-S66": 2,
		LOCALSLAVEID:                               3,
		"20160107-001256-134875658-5050-27524-S1":  3,
		"20160107-001256-134875658-5050-27524-S2":  2,
		"20160107-001256-134875658-5050-27524-S0":  2,
	}

	// 5 Records ( 5x slaves )
	validateStateRecords(t, backend.TaskRecords, 5, expected)
}

func TestHealthchecks(t *testing.T) {
	server, backend := recordSetup(t)
	defer server.Stop()

	setupHealthChecks(t, backend)

	// Need to do this to populate backend.SlaveIDIP
	// so we can pull appropriate slave ip mapping
	// by slave.ID
	backend.generateMesosRecords()

	for _, framework := range backend.State.Frameworks {
		backend.generateTaskRecords(framework.Tasks)
	}

	// Each slave can have a different number of tasks running
	expected := map[string]int{
		"20160107-001256-134875658-5050-27524-S66": 2,
		LOCALSLAVEID:                               3,
		"20160107-001256-134875658-5050-27524-S1":  3,
		"20160107-001256-134875658-5050-27524-S2":  2,
		"20160107-001256-134875658-5050-27524-S0":  2,
	}

	// 5 Records ( 5x slaves )
	validateStateRecords(t, backend.TaskRecords, 5, expected)
	// 2 healthchecks
	validateHealthRecords(t, backend.HealthChecks[LOCALSLAVEID].Current, 2)
}

func TestRegister(t *testing.T) {
	server, backend := recordSetup(t)
	defer server.Stop()

	setupHealthChecks(t, backend)
	// Need to do this to populate backend.SlaveIDIP
	// so we can pull appropriate slave ip mapping
	// by slave.ID
	backend.generateMesosRecords()
	backend.generateFrameworkRecords()

	for _, framework := range backend.State.Frameworks {
		// Do a little jiggling of the handle
		// to add healthchecks to our local(127.0.0.1) task
		for _, task := range framework.Tasks {
			if task.ID == "nginx-no-port.4266d369-b9a7-11e5-b2bb-0242d4d0a230" {
				//t.Log("Adding nginx/port healthcheck for", task.ID)
				label := state.Label{
					Key:   "ConsulHealthCheckKeys",
					Value: "nginx/port,nginx/http",
				}

				task.Labels = append(task.Labels, label)
			}
		}
		backend.generateTaskRecords(framework.Tasks)
	}
	backend.Register()

	// 6 Records ( 2x myapp, 1x nginx, 1x marathon, 1x slave, 1x consul )
	validateRecords(t, backend, 6)

}

func TestCleanupRecords(t *testing.T) {
	server, backend := recordSetup(t)
	defer server.Stop()

	setupHealthChecks(t, backend)

	rg := &records.RecordGenerator{State: backend.State}
	backend.Reload(rg)
	validateRecords(t, backend, 6)
	validateHealthRecords(t, backend.HealthChecks[LOCALSLAVEID].Previous, 2)

	service := createService("REMOVEMESERVICE", "REMOVEMESERVICE", "127.0.0.2", 0, []string{})
	err := backend.Agents[LOCALSLAVENAME].ServiceRegister(service)
	if err != nil {
		t.Error("Failed to create bogus service", err)
	}
	validateRecords(t, backend, 7)

	slaveid := backend.SlaveHostnameID[LOCALSLAVENAME]
	// Add this to previous so when we parse state again,
	// this will not be present
	backend.TaskRecords[slaveid].Previous = append(backend.TaskRecords[slaveid].Previous, service)

	backend.Reload(rg)
	validateRecords(t, backend, 6)

	hc := &capi.AgentCheckRegistration{
		ID:                "REMOVEMECHECK",
		Name:              "REMOVEMECHECK",
		ServiceID:         "mesos-dns:mesosslave-r02-s02:nginx-no-port.4266d369-b9a7-11e5-b2bb-0242d4d0a230",
		AgentServiceCheck: capi.AgentServiceCheck{TTL: "500s"},
	}
	err = backend.Agents[LOCALSLAVENAME].CheckRegister(hc)
	if err != nil {
		t.Error("Failed to create bogus healthcheck", err)
	}

	backend.HealthChecks[slaveid].Previous = append(backend.HealthChecks[slaveid].Previous, hc)
	backend.Reload(rg)

	// Need to do some sort of validation for ^
	validateHealthRecords(t, backend.HealthChecks[LOCALSLAVEID].Previous, 2)

	// Test for updating existing HC
	kv := backend.Client.KV()
	nport := &capi.AgentCheckRegistration{
		ID:   "nginx/port",
		Name: "nginx/port",
		AgentServiceCheck: capi.AgentServiceCheck{
			TCP:      "{IP}:800",
			Interval: "5s",
		},
	}
	b, err := json.Marshal(nport)
	p := &capi.KVPair{Key: "healthchecks/" + nport.ID, Value: b}
	_, err = kv.Put(p, nil)
	if err != nil {
		t.Error(err)
	}

	//err = backend.Agents[LOCALSLAVENAME].CheckRegister(nport)
	//t.Log(backend.Agents[LOCALSLAVENAME].Health().Checks("nginx-no-port", nil))
	backend.Reload(rg)
	validateHealthRecords(t, backend.HealthChecks[LOCALSLAVEID].Previous, 2)
	backend.Reload(rg)
	validateHealthRecords(t, backend.HealthChecks[LOCALSLAVEID].Previous, 2)
}

func TestCache(t *testing.T) {
	server, backend := recordSetup(t)
	defer server.Stop()

	// Need to do this to populate backend.SlaveIDIP
	// so we can pull appropriate slave ip mapping
	// by slave.ID
	setupHealthChecks(t, backend)

	rg := &records.RecordGenerator{State: backend.State}
	backend.Reload(rg)

	// 6 Records ( 2x myapp, 1x nginx, 1x marathon, 1x slave, 1x consul )
	validateRecords(t, backend, 6)

	// Save us uglyness later
	slaveid := backend.SlaveHostnameID[LOCALSLAVENAME]

	// Create new service
	service := createService(strings.Join([]string{backend.Config.ServicePrefix, "REMOVEMESERVICE"}, ":"), "REMOVEMESERVICE", "127.0.0.1", 0, []string{})
	// Add a new service to current
	backend.TaskRecords[slaveid].Current = append(backend.TaskRecords[slaveid].Current, service)
	// Compare
	delta := getDeltaServices(backend.TaskRecords[slaveid].Previous, backend.TaskRecords[slaveid].Current)
	if len(delta) != 1 {
		t.Error("Did not get back additional service registration. Expected 1 received", len(delta))
	}

	// Create new healthcheck
	hc := &capi.AgentCheckRegistration{
		ID:                "REMOVEMECHECK2",
		Name:              "REMOVEMECHECK2",
		ServiceID:         "mesos-dns:mesosslave-r02-s02:nginx-no-port.4266d369-b9a7-11e5-b2bb-0242d4d0a230",
		AgentServiceCheck: capi.AgentServiceCheck{TTL: "500s"},
	}

	// Add in new healthcheck to current
	backend.HealthChecks[slaveid].Current = append(backend.HealthChecks[slaveid].Current, hc)

	// Compare
	deltachecks := getDeltaChecks(backend.HealthChecks[slaveid].Previous, backend.HealthChecks[slaveid].Current, "add")
	if len(deltachecks) != 1 {
		t.Error("Did not get back additional healthcheck registration. Expected 1 received", len(deltachecks))
	}

	// Do some juggling for tests
	save := backend.TaskRecords[slaveid].Previous

	// Go ahead and register our new service
	backend.Register()
	validateRecords(t, backend, 7)

	// Restore our saved previous information
	// so we can essentiall orphan `service`
	backend.TaskRecords[slaveid].Previous = save
	backend.TaskRecords[slaveid].Current = nil

	backend.Count = backend.Config.CacheRefresh - 1
	// Clean up our mess
	backend.Reload(rg)
	validateRecords(t, backend, 6)
}

func makeClientServer(t *testing.T) *testutil.TestServer {

	// Make client config
	conf := capi.DefaultConfig()

	// Create server
	// Redirect logs to /dev/null cause we really dont care about consul agent ouput
	server := testutil.NewTestServerConfig(t, func(c *testutil.TestServerConfig) {
		c.NodeName = LOCALSLAVENAME
		c.LogLevel = "ERR"
		c.Stdout = ioutil.Discard
		c.Stderr = ioutil.Discard
	})
	conf.Address = server.HTTPAddr

	return server
}

func loadState(t *testing.T) state.State {
	var sj state.State
	b, err := ioutil.ReadFile("test/state.json")
	if err != nil {
		t.Fatal(err)
	} else if err = json.Unmarshal(b, &sj); err != nil {
		t.Fatal(err)
	}

	return sj
}

func backendSetup(t *testing.T) (*testutil.TestServer, *ConsulBackend) {
	server := makeClientServer(t)

	// Let's try to have fun with consul config
	os.Setenv("CONSUL_HTTP_ADDR", server.HTTPAddr)
	defer os.Setenv("CONSUL_HTTP_ADDR", "")

	config := NewConfig()
	config.Address = server.HTTPAddr
	errch := make(chan error)
	version := "1.0"

	// Initialize logger
	//logging.VeryVerboseFlag = true
	logging.SetupLogs()

	// Create empty RecordGenerator
	conf := records.NewConfig()
	conf.StateTimeoutSeconds = 300
	rg := records.NewRecordGenerator(conf)

	// Hopefully the ENV vars above should allow us
	// to override the defaults
	backend := New(config, errch, rg, version)
	_, err := backend.Client.Agent().Self()
	if err != nil {
		t.Error("Failed to get consul client initialized")
	}
	return server, backend
}

func recordSetup(t *testing.T) (*testutil.TestServer, *ConsulBackend) {
	sj := loadState(t)

	server, backend := backendSetup(t)
	err := backend.connectAgents()
	if err != nil {
		t.Error("Issue connecting to agents.", err)
	}

	// :D
	// Do this for testing so we can have
	// a consul agent ( our dummy test server )
	// running on the same host as the mesos process
	for _, slave := range sj.Slaves {
		if slave.ID == LOCALSLAVEID {
			slave.PID.Host = LOCALSLAVEIP
		}
	}
	sj.Leader = strings.Join([]string{"master@", LOCALSLAVEIP, ":5050"}, "")
	sj.Frameworks[0].PID.Host = LOCALSLAVEIP
	backend.State = sj

	return server, backend
}

func validateRecords(t *testing.T, backend *ConsulBackend, expected int) {
	for _, agent := range backend.Agents {
		services, err := agent.Services()
		if err != nil {
			t.Error("Unable to get list of services back from agent.", err)
			return
		}

		if len(services) != expected {
			t.Error("Did not get back", expected, "services. Got back", len(services))
			t.Error("Services:")
			for k, info := range services {
				t.Error(" -", k, "=>", info.Address)
			}
		}

		// Test local agent for leader.mesos record
		leader := false
		for _, tag := range services[strings.Join([]string{"mesos-dns", LOCALSLAVEID}, ":")].Tags {
			if tag == "leader" {
				leader = true
			}
		}
		if !leader {
			t.Error("leader tag not applied to local slave")
		}
	}
}

func validateChecks(t *testing.T, backend *ConsulBackend, expected int) {
	for _, agent := range backend.Agents {
		checks, err := agent.Checks()
		if err != nil {
			t.Error(err)
		}

		if len(checks) != expected {
			t.Error("Did not get back", expected, "checks. Got back", len(checks))
			t.Error("Checks:")
			for k, info := range checks {
				t.Error(" -", k, "=>", info.ServiceID)
			}
		}
	}
}

func validateStateRecords(t *testing.T, records map[string]*ConsulRecords, expectedrecs int, expected map[string]int) {
	if len(records) != expectedrecs {
		t.Error("Did not get back", expectedrecs, "records. Got back", len(records))
		for id := range records {
			t.Error("-", id)
		}
		return
	}

	for id, asr := range records {
		// Success
		if len(asr.Current) == expected[id] {
			continue
		}

		// Failure
		t.Error("Did not get back", expected[id], "state records. Got back", len(asr.Current))
		t.Error(id)
		for _, info := range asr.Current {
			t.Error(" -", info.ID, info.Name, info.Address)
		}
	}
}

func validateHealthRecords(t *testing.T, records []*capi.AgentCheckRegistration, expected int) {
	if len(records) != expected {
		t.Error("Did not get back", expected, "healthcheck records. Got back", len(records))
		for _, hc := range records {
			t.Error(" -", hc.ServiceID)
		}
		return
	}

	for _, info := range records {
		if info.Name == "nginx/port" {
			if strings.Split(info.AgentServiceCheck.TCP, ":")[0] != "127.0.0.1" {
				t.Errorf("IP Substitution did not work %+v", info)
			}
		}
	}

}

func setupHealthChecks(t *testing.T, backend *ConsulBackend) {
	// Post KV for consul healthchecks
	// nginx/port
	// nginx/http
	kv := backend.Client.KV()
	nport := &capi.AgentCheckRegistration{
		ID:   "nginx/port",
		Name: "nginx/port",
		AgentServiceCheck: capi.AgentServiceCheck{
			TCP:      "{IP}:80",
			Interval: "5s",
		},
	}

	nhttp := &capi.AgentCheckRegistration{
		ID:   "nginx/http",
		Name: "nginx/http",
		AgentServiceCheck: capi.AgentServiceCheck{
			HTTP:     "http://localhost",
			Interval: "5s",
		},
	}

	for _, check := range []*capi.AgentCheckRegistration{nport, nhttp} {
		b, err := json.Marshal(check)
		p := &capi.KVPair{Key: "healthchecks/" + check.ID, Value: b}
		_, err = kv.Put(p, nil)
		if err != nil {
			t.Error(err)
		}
	}

}
