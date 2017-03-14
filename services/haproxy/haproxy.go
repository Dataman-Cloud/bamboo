package haproxy

import (
	"errors"
	"fmt"
	"log"
	"strings"

	conf "github.com/QubitProducts/bamboo/configuration"
	"github.com/QubitProducts/bamboo/services/application"
	"github.com/QubitProducts/bamboo/services/marathon"
)

type templateData struct {
	Frontends     []Frontend
	HaproxyUiPort string
}

type Server struct {
	Name           string
	Version        string
	Host           string
	Port           int
	Weight         int
	BackendMaxConn string
}

type ByVersion []Server

func (a ByVersion) Len() int {
	return len(a)
}
func (a ByVersion) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a ByVersion) Less(i, j int) bool {
	return a[i].Version < a[j].Version
}

type Frontend struct {
	Name     string
	Protocol string
	Bind     string
	Servers  []Server
}

func GetTemplateData(config *conf.Configuration) (*templateData, error) {
	frontends, err := formFrontends(config)
	if err != nil {
		return nil, err
	}
	log.Printf("frontends = %+v", frontends)

	haproxyUiPort := config.HAProxy.UiPort

	return &templateData{frontends, haproxyUiPort}, nil
}

func formFrontends(config *conf.Configuration) ([]Frontend, error) {
	marathonApp, err := marathon.FetchMarathonApp(config)
	if err != nil {
		return nil, err
	}

	fmt.Printf("marathonApp ===== %+v", marathonApp)

	haproxy_port := marathonApp.Env["HAPROXY_PORT"]
	use_macvlan := marathonApp.Env["USE_MACVLAN"]
	haproxy_port_slice := strings.Split(haproxy_port, ":")
	if len(haproxy_port_slice) != 2 {
		return nil, errors.New("the HAPROXY_PORT env must be protocol with port like tcp:7788")
	}

	servers := []Server{}
	for _, task := range marathonApp.Tasks {
		var server Server
		if use_macvlan == "true" {
			for _, port := range marathonApp.IpAddress.Discovery.Ports {
				for _, ipaddress := range task.IpAddresses {
					server = Server{
						Name:           fmt.Sprintf("%s-%d", task.IpAddresses[0].IpAddress, port.Number),
						Host:           ipaddress.IpAddress,
						Port:           port.Number,
						BackendMaxConn: config.HAProxy.BackendMaxConn,
					}
					servers = append(servers, server)
				}
			}

		} else {
			for _, port := range task.Ports {
				server = Server{
					Name:           fmt.Sprintf("%s-%d", task.Host, port),
					Host:           task.Host,
					Port:           port,
					BackendMaxConn: config.HAProxy.BackendMaxConn,
				}
				servers = append(servers, server)
			}
		}
	}

	var frontends []Frontend
	frontend := Frontend{
		Name:     fmt.Sprintf("%s-%s-%s", config.Application.Id, haproxy_port_slice[0], haproxy_port_slice[1]),
		Protocol: haproxy_port_slice[0],
		Bind:     haproxy_port_slice[1],
		Servers:  servers,
	}
	frontends = append(frontends, frontend)
	return frontends, nil
}

func formServers(frontend Frontend, weights map[string][2]int) []map[string]interface{} {
	servers := []map[string]interface{}{}
	for _, server := range frontend.Servers {
		weight := weights[server.Version]
		w, r := weight[0], weight[1]
		//only use remainder on first server
		if r > 0 {
			newWeight := weight
			newWeight[1] = 0
			weights[server.Version] = newWeight
		}
		svr := map[string]interface{}{
			"backend": frontend.Name,
			"server":  server.Name,
			"weight":  w + r,
		}
		servers = append(servers, svr)
	}
	return servers
}

func formVersionWeights(weight application.Weight, versionMap map[string][]Server) map[string][2]int {
	weights := map[string][2]int{}
	for vsn, servers := range versionMap {
		len := len(servers)
		exactWeight := weight.Versions[vsn] / len
		remainder := weight.Versions[vsn] % len
		weights[vsn] = [2]int{exactWeight, remainder}
	}
	return weights
}

func formVersionMap(frontend Frontend) map[string][]Server {
	versions := map[string][]Server{}
	for _, server := range frontend.Servers {
		servers, ok := versions[server.Version]
		if ok {
			servers = append(servers, server)
		} else {
			servers = []Server{server}
		}
		versions[server.Version] = servers
	}
	return versions
}
