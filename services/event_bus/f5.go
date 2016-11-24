package event_bus

import (
	"github.com/QubitProducts/bamboo/configuration"
)

type F5 struct {
	Config *configuration.Configuration
}

type VirtualServer struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Destination string `json:"destination"`
	IpProtocol  string `json:"ipProtocol"`
	Pool        string `json:"pool"`
}

type Pool struct {
	Name              string   `json:"name"`
	LoadBalancingMode string   `json:"loadBalancingMode"`
	Monitor           string   `json:"monitor"`
	Members           []Member `json:"members"`
}

type Member struct {
	Address string `json:"address"`
	Name    string `json:"name"`
}
