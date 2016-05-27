---
title: Mesos-DNS Configuration Parameters
---

##  Mesos-DNS Configuration Parameters

Mesos-DNS is configured through the parameters in a json file. You can point Mesos-DNS to a specific configuration file using the argument `-config=pathto/file.json`. If no configuration file is passed as an argument, Mesos-DNS will look for file `config.json` in the current directory. 

The configuration file should include the following fields:

```
{
  "Domain": "mesos",
  "IPSources": ["netinfo", "mesos", "host"]
  "Masters": ["10.101.160.15:5050", "10.101.160.16:5050", "10.101.160.17:5050"],
  "Port": 53,
  "RefreshSeconds": 60,
  "Resolvers": {
    "builtin": {
      "DNsOn": true,
      "ExternalDNS": ["8.8.8.8"]
      "ExternalOn": true,
      "HTTPOn": true,
      "HTTPPort": 8123,
      "Listener": "10.101.160.16",
      "Timeout": 5, 
      "TTL": 60
    }
  },
  "SOAMname": "ns1.mesos",
  "SOARname": "root.ns1.mesos",
  "SOARefresh": 60,
  "SOARetry":   600,
  "SOAExpire":  86400,
  "SOAMinttl": 60,
  "ZK": "zk://10.101.160.15:2181/mesos",
}
```
### Top-level Parameters

`Masters` is a comma separated list with the IP address and port number for the master(s) in the Mesos cluster. Mesos-DNS will automatically find the leading master at any point in order to retrieve state about running tasks. If there is no leading master or the leading master is not responsive, Mesos-DNS will continue serving DNS requests based on stale information about running tasks. The `Masters` field is required. 

It is sufficient to specify just one of the `ZK` or `Masters` field. If both are defined, Mesos-DNS will first attempt to detect the leading master through Zookeeper. If Zookeeper is not responding, it will fall back to using the `Masters` field. Both `ZK` and `Master` fields are static. To update them you need to restart Mesos-DNS. We recommend you use the `ZK` field since this allows the dynamic addition to Mesos masters. 

`RefreshSeconds` is the frequency at which Mesos-DNS updates DNS records based on information retrieved from the Mesos master. The default value is 60 seconds. 

`StateTimeoutSeconds` is the time that Mesos-DNS will wait for the Mesos master to respond to its request for state.json in seconds. The default value is 300 seconds.

`Domain` is the domain name for the Mesos cluster. The domain name can use characters [a-z, A-Z, 0-9], `-` if it is not the first or last character of a domain portion, and `.` as a separator of the textual portions of the domain name. We recommend you avoid valid [top-level domain names](http://en.wikipedia.org/wiki/List_of_Internet_top-level_domains). The default value is `mesos`.

`SOAMname` specifies the domain name of the name server that was the original or primary source of data for the configured domain.
The configured name will always be converted to a FQDN by ensuring it ends with a `.`. The default value is `ns1.mesos`.

`SOARname` specifies the mailbox of the person responsible for the configured domain. The format is `mailbox.domain`, using a `.` instead of `@`. i.e. `root@ns1.mesos` becomes `root.ns1.mesos`. For details, see the [RFC-1035](http://tools.ietf.org/html/rfc1035#page-18). The default value is `root.ns1.mesos`.

`SOARefresh` is the REFRESH field in the SOA record for the Mesos domain. For details, see the [RFC-1035](http://tools.ietf.org/html/rfc1035#page-18). The default value is `60`.

`SOARetry` is the RETRY field in the SOA record for the Mesos domain. For details, see the [RFC-1035](http://tools.ietf.org/html/rfc1035#page-18). The default value is `600`.

`SOAExpire` is the EXPIRE field in the SOA record for the Mesos domain. For details, see the [RFC-1035](http://tools.ietf.org/html/rfc1035#page-18). The default value is `86400`.

`SOAMinttl` is the minimum TTL field in the SOA record for the Mesos domain. For details, see the [RFC-2308](https://tools.ietf.org/html/rfc2308). The default value is `60`.

`enforceRFC952` will enforce an older, more strict set of rules for DNS labels. For details, see the [RFC-952](https://tools.ietf.org/html/rfc952). The default value is `false`.

`IPSources` defines a fallback list of IP sources for task records,
sorted by priority. If you use **Docker**, and enable the `netinfo` IPSource, it may cause tasks to become unreachable, because after Mesos 0.25, the Docker executor publishes the container's internal IP in NetworkInfo. The default value is: `["netinfo", "mesos", "host"]`

`ZK` is a link to the Zookeeper instances on the Mesos cluster. Its format is `zk://host1:port1,host2:port2/mesos/`, where the number of hosts can be one or more. The default port for Zookeeper is `2181`. Mesos-DNS will monitor the Zookeeper instances to detect the current leading master. 

`ZKDetectionTimeout` defines how long to wait (in seconds) for Zookeeper to report a new leading Mesos master.
This timeout is activated on:

- Start up, where it plays the role of the "initial leader detection timeout" via ZK.
- Mesos cluster changes, where there is no leading master for some period of time.
- Zookeeper or network failure, when losing connection to the ZK cluster.

If a *non-zero* timeout is specified and the timeout threshold is exceeded before
a new leading Mesos master is reported by the ZK-based master detector, the program will exit.

Defaults to `30` seconds.

### Builtin Resolver Parameters

To use the `builtin` resolver, a `builtin` key must be listed under `Resolvers`. The value for `builtin` can be empty, which will cause the default settings to be used.

`DNSOn` is a boolean field that controls whether Mesos-DNS listens for DNS requests or not. The default value is `true`. 

`ExternalDNS` is a comma separated list with the IP addresses of external DNS servers that Mesos-DNS will contact to resolve any DNS requests outside the `domain`. We ***recommend*** that you list the nameservers specified in the `/etc/resolv.conf` on the server Mesos-DNS is running. Alternatively, you can list `8.8.8.8`, which is the [Google public DNS](https://developers.google.com/speed/public-dns/) address.
 
`ExternalOn` is a boolean field that controls whether Mesos-DNS serves requests outside of the Mesos domain. The default value is `true`. 

`HTTPOn` is a boolean field that controls whether Mesos-DNS listens for HTTP requests or not. The default value is `true`. 

`HTTPPort` is the port number that Mesos-DNS monitors for incoming HTTP requests. The default value is `8123`.

`Listener` is the IP address of Mesos-DNS. In SOA replies, Mesos-DNS identifies hostname `mesos-dns.domain` as the primary nameserver for the domain. It uses this IP address in an A record for `mesos-dns.domain`. The default value is "0.0.0.0", which instructs Mesos-DNS to create an A record for every IP address associated with a network interface on the server that runs the Mesos-DNS process. 

`Port` is the port number that Mesos-DNS monitors for incoming DNS requests. Requests can be sent over TCP or UDP. We recommend you use port `53` as several applications assume that the DNS server listens to this port. The default value is `53`.

`RecurseOn` controls if the DNS replies for names in the Mesos domain will indicate that recursion is available. The default value is `true`. 

`Timeout` is the timeout threshold, in seconds, for connections and requests to external DNS requests. The default value is 5 seconds. 

`TTL` is the [time to live](http://en.wikipedia.org/wiki/Time_to_live#DNS_records) value for DNS records served by Mesos-DNS, in seconds. It allows caching of the DNS record for a period of time in order to reduce DNS request rate. `ttl` should be equal or larger than `RefreshSeconds`. The default value is 60 seconds. 

### Alternative Resolver Parameters

These settings are highly dependent on whatever resolvers one is trying to use. As with the `builtin` resolver, simply including the name of the resolver in the `Resolvers` field value will tell Mesos-DNS to try and load it. Leaving the key/value settings under that name empty will have Mesos-DNS load whatever default config is provided by the plugin. 

- `host`: Host IP of the Mesos slave where a task is running.
- `mesos`: Mesos containerizer IP. **DEPRECATED**
- `docker`: Docker containerizer IP. **DEPRECATED**
- `netinfo`: Mesos 0.25 NetworkInfo.
