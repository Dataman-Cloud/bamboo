package haproxy

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"

	conf "github.com/QubitProducts/bamboo/configuration"
	"github.com/QubitProducts/bamboo/services/application"
	"github.com/QubitProducts/bamboo/services/marathon"
	"github.com/QubitProducts/bamboo/services/service"
)

type templateData struct {
	Frontends   []Frontend
	Services    map[string]service.Service
	HaproxyPort string
}

type Server struct {
	Name    string
	Version string
	Host    string
	Port    int
	Weight  int
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
	Bind     int
	Servers  []Server
}
type ByBind []Frontend

func (a ByBind) Len() int {
	return len(a)
}
func (a ByBind) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a ByBind) Less(i, j int) bool {
	return a[i].Bind < a[j].Bind
}

var FrontendMap map[string]Frontend = make(map[string]Frontend)

func GetTemplateData(config *conf.Configuration, storage service.Storage, appStorage application.Storage) (*templateData, error) {
	apps, err := marathon.FetchApps(config.Marathon, config)

	if err != nil {
		return nil, err
	}

	services, err := storage.All()
	if err != nil {
		return nil, err
	}

	frontends := formFrontends(apps)
	log.Printf("frontends = %+v", frontends)

	byName := make(map[string]service.Service)
	for _, service := range services {
		byName[service.Id] = service
	}
	haproxyPort := config.HAProxy.UiPort
	return &templateData{frontends, byName, haproxyPort}, nil
}

func formWeightMap(zkWeights []application.Weight) map[string]int {
	weightMap := map[string]int{}
	processed := map[string]bool{}
	for _, weight := range zkWeights {
		if frontend, ok := FrontendMap[weight.ID]; ok {
			servers := CalcWeights(frontend, weight)
			for _, server := range servers {
				weightMap[server["server"].(string)] = server["weight"].(int)
			}
			processed[weight.ID] = true
		}
	}
	//set initial weight
	for id, frontend := range FrontendMap {
		if !processed[id] {
			for _, server := range frontend.Servers {
				weightMap[server.Name] = server.Weight
			}
		}
	}
	return weightMap
}

func formFrontends(apps marathon.AppList) []Frontend {
	frontends := []Frontend{}
	for _, app := range apps {
		endpointsLen := len(app.Endpoints)
		if endpointsLen > 0 {
			for epIdx, endpoint := range app.Endpoints {
				frontend := Frontend{
					Name:     fmt.Sprintf("%s-%s-%d", app.Frontend, endpoint.Protocol, endpoint.Bind),
					Protocol: endpoint.Protocol,
					Bind:     endpoint.Bind,
				}

				servers := []Server{}
				for _, task := range app.Tasks {
					// endpoint contain haproxy map port so endpoint must one-one correspondence task.Port
					// then length of endpoint must be equal to length task.Port
					if len(task.Ports) != endpointsLen {
						continue
					}
					server := Server{
						Name: fmt.Sprintf("%s-%d", task.Server, task.Ports[epIdx]),
						Host: task.Host,
						Port: task.Ports[epIdx],
					}
					servers = append(servers, server)
				}
				sort.Sort(ByVersion(servers))
				frontend.Servers = servers

				frontends = append(frontends, frontend)
				FrontendMap[app.Id] = frontend
			}
		}
	}
	sort.Sort(ByBind(frontends))
	return frontends
}

func handleCanary(apps marathon.AppList, weights []application.Weight) (result marathon.AppList) {
	weightMap := extractWeights(weights)
	weightMapJson, _ := json.Marshal(weightMap)
	log.Println("weightMap", string(weightMapJson))
	result = marathon.AppList{}
	for _, app := range apps {
		weight, hasWeight := weightMap[app.Id]
		log.Println("weight", weight, "hasWeight", hasWeight)
		newTasks := []marathon.Task{}
		for _, task := range app.Tasks {
			if task.Version == app.CurVsn {
				task.Weight = 1
			} else {
				task.Weight = 0
			}
			log.Println("task version", task.Version, "curVsn", app.CurVsn)
			log.Println("task weight", task.Weight)
			newTasks = append(newTasks, task)
		}
		app.Tasks = newTasks
		result = append(result, app)
	}
	return result
}

func extractWeights(weights []application.Weight) map[string]application.Weight {
	weightMap := make(map[string]application.Weight, len(weights))
	for _, weight := range weights {
		weightMap[weight.ID] = weight
	}

	return weightMap
}

//CalcWeights clac server weights
func CalcWeights(frontend Frontend, weight application.Weight) []map[string]interface{} {
	versionMap := formVersionMap(frontend)
	versionMapJson, _ := json.Marshal(versionMap)
	log.Println("versionMap", string(versionMapJson))

	versionWeights := formVersionWeights(weight, versionMap)
	versionWeightsJson, _ := json.Marshal(versionWeights)
	log.Println("versionWeights", string(versionWeightsJson))

	servers := formServers(frontend, versionWeights)
	serversJson, _ := json.Marshal(servers)
	log.Println("servers", string(serversJson))

	return servers
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
